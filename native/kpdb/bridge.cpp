#include "bridge.h"

#include <algorithm>
#include <cstdlib>
#include <cstring>
#include <limits>
#include <memory>
#include <mutex>
#include <stdexcept>

#include <QBuffer>
#include <QByteArray>
#include <QCoreApplication>
#include <QDateTime>
#include <QFile>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QString>

#include "core/Database.h"
#include "core/CustomData.h"
#include "core/Entry.h"
#include "core/EntryAttachments.h"
#include "core/EntryAttributes.h"
#include "core/Group.h"
#include "core/Metadata.h"
#include "crypto/Crypto.h"
#include "crypto/kdf/Argon2Kdf.h"
#include "format/KeePass2.h"
#include "format/KeePass2Reader.h"
#include "format/KeePass2Writer.h"
#include "keys/CompositeKey.h"
#include "keys/FileKey.h"
#include "keys/PasswordKey.h"

namespace {

std::once_flag init_once;
bool crypto_ready = false;
QString crypto_error;
std::unique_ptr<QCoreApplication> application;

QByteArray bytes(const uint8_t* value, size_t length)
{
    if (value == nullptr || length == 0) {
        return {};
    }
    if (length > static_cast<size_t>(std::numeric_limits<int>::max())) {
        throw std::runtime_error("native input is too large");
    }
    return QByteArray(reinterpret_cast<const char*>(value), static_cast<int>(length));
}

QString text(const char* value, size_t length)
{
    return QString::fromUtf8(bytes(reinterpret_cast<const uint8_t*>(value), length));
}

QJsonObject object(const uint8_t* value, size_t length, const char* label)
{
    QJsonParseError error;
    const auto document = QJsonDocument::fromJson(bytes(value, length), &error);
    if (error.error != QJsonParseError::NoError || !document.isObject()) {
        throw std::runtime_error(QString("invalid %1 JSON: %2").arg(label, error.errorString()).toStdString());
    }
    return document.object();
}

void initialize()
{
    std::call_once(init_once, [] {
        if (QCoreApplication::instance() == nullptr) {
            static int argc = 1;
            static char name[] = "bwkp";
            static char* argv[] = {name, nullptr};
            application = std::make_unique<QCoreApplication>(argc, argv);
        }
        crypto_ready = Crypto::init();
        if (!crypto_ready) {
            crypto_error = Crypto::errorString();
        }
    });
    if (!crypto_ready) {
        throw std::runtime_error(QString("KeePassXC crypto initialization failed: %1").arg(crypto_error).toStdString());
    }
}

QDateTime timestamp(const QJsonValue& value)
{
    if (!value.isString() || value.toString().isEmpty()) {
        return {};
    }
    const auto result = QDateTime::fromString(value.toString(), Qt::ISODateWithMs);
    if (!result.isValid()) {
        throw std::runtime_error(QString("invalid RFC3339 timestamp: %1").arg(value.toString()).toStdString());
    }
    return result;
}

void set_times(Entry* entry, const QJsonObject& source)
{
    auto info = entry->timeInfo();
    if (const auto value = timestamp(source.value("created")); value.isValid()) {
        info.setCreationTime(value);
    }
    if (const auto value = timestamp(source.value("modified")); value.isValid()) {
        info.setLastModificationTime(value);
    }
    if (const auto value = timestamp(source.value("accessed")); value.isValid()) {
        info.setLastAccessTime(value);
    }
    if (const auto value = timestamp(source.value("expires")); value.isValid()) {
        info.setExpiryTime(value);
        info.setExpires(true);
    }
    entry->setTimeInfo(info);
}

Entry* make_entry(const QJsonObject& source)
{
    auto entry = std::make_unique<Entry>();
    entry->setTitle(source.value("title").toString());
    entry->setUsername(source.value("username").toString());
    entry->setPassword(source.value("password").toObject().value("value").toString());
    entry->setUrl(source.value("url").toString());
    entry->setNotes(source.value("notes").toString());

    QStringList tags;
    for (const auto& value : source.value("tags").toArray()) {
        tags.append(value.toString());
    }
    entry->setTags(tags.join(','));

    const auto fields = source.value("fields").toObject();
    for (auto field = fields.begin(); field != fields.end(); ++field) {
        const auto data = field.value().toObject();
        entry->attributes()->set(field.key(), data.value("value").toString(), data.value("protected").toBool());
    }
    for (const auto& value : source.value("attachments").toArray()) {
        const auto attachment = value.toObject();
        entry->attachments()->set(attachment.value("name").toString(),
                                  QByteArray::fromBase64(attachment.value("content").toString().toLatin1()));
    }
    set_times(entry.get(), source);
    for (const auto& value : source.value("history").toArray()) {
        entry->addHistoryItem(make_entry(value.toObject()));
    }
    return entry.release();
}

Group* make_group(const QJsonObject& source)
{
    auto group = std::make_unique<Group>();
    group->setName(source.value("name").toString());
    for (const auto& value : source.value("groups").toArray()) {
        auto child = make_group(value.toObject());
        child->setParent(group.get());
    }
    for (const auto& value : source.value("entries").toArray()) {
        auto entry = make_entry(value.toObject());
        entry->setGroup(group.get());
    }
    return group.release();
}

QSharedPointer<CompositeKey> key(const QJsonObject& credentials)
{
    auto result = QSharedPointer<CompositeKey>::create();
    const auto password = QByteArray::fromBase64(credentials.value("password").toString().toLatin1());
    if (!password.isEmpty()) {
        result->addKey(QSharedPointer<PasswordKey>::create(QString::fromUtf8(password)));
    }
    auto key_file = QByteArray::fromBase64(credentials.value("keyFile").toString().toLatin1());
    if (!key_file.isEmpty()) {
        QBuffer buffer(&key_file);
        buffer.open(QIODevice::ReadOnly);
        auto file_key = QSharedPointer<FileKey>::create();
        QString error;
        if (!file_key->load(&buffer, &error)) {
            throw std::runtime_error(QString("invalid KeePass key file: %1").arg(error).toStdString());
        }
        result->addKey(file_key);
    }
    if (result->isEmpty()) {
        throw std::runtime_error("a database password or key file is required");
    }
    return result;
}

void configure(Database& database, const QJsonObject& options)
{
    const auto cipher = options.value("Cipher").toString();
    if (cipher == "aes256") {
        database.setCipher(KeePass2::CIPHER_AES256);
    } else if (cipher == "chacha20") {
        database.setCipher(KeePass2::CIPHER_CHACHA20);
    } else {
        throw std::runtime_error(QString("unsupported KDBX cipher: %1").arg(cipher).toStdString());
    }
    const auto compression = options.value("Compression").toString();
    if (compression == "gzip") {
        database.setCompressionAlgorithm(Database::CompressionGZip);
    } else if (compression == "none") {
        database.setCompressionAlgorithm(Database::CompressionNone);
    } else {
        throw std::runtime_error(QString("unsupported KDBX compression: %1").arg(compression).toStdString());
    }
    if (options.value("KDF").toString() != "argon2id") {
        throw std::runtime_error("only Argon2id is supported");
    }
    auto kdf = QSharedPointer<Argon2Kdf>::create(Argon2Kdf::Type::Argon2id);
    if (!kdf->setMemory(static_cast<quint64>(options.value("MemoryKiB").toDouble()))
        || !kdf->setParallelism(options.value("Parallelism").toInt())) {
        throw std::runtime_error("invalid Argon2id resource parameters");
    }
    const auto configured = static_cast<qint64>(options.value("Iterations").toDouble());
    const auto target_ns = static_cast<qint64>(options.value("TargetTime").toDouble());
    const auto rounds = configured > 0 ? configured : kdf->benchmark(static_cast<int>(target_ns / 1000000));
    if (rounds <= 0 || rounds > std::numeric_limits<int>::max() || !kdf->setRounds(static_cast<int>(rounds))) {
        throw std::runtime_error("invalid Argon2id iteration count");
    }
    kdf->randomizeSeed();
    database.setKdf(kdf);
    database.setFormatVersion(KeePass2::FILE_VERSION_4_1);
}

void write_database(const QString& path,
                    const QJsonObject& model,
                    const QJsonObject& credentials,
                    const QJsonObject& options)
{
    initialize();
    Database database;
    database.metadata()->setName(model.value("name").toString());
    std::unique_ptr<Group> previous_root(database.setRootGroup(make_group(model.value("root").toObject())));
    configure(database, options);
    const auto credentials_key = key(credentials);
    if (!database.setKey(credentials_key)) {
        throw std::runtime_error(QString("set database key: %1").arg(database.keyError()).toStdString());
    }
    QFile output(path);
    if (!output.open(QIODevice::WriteOnly | QIODevice::Truncate)) {
        throw std::runtime_error(QString("open KDBX output: %1").arg(output.errorString()).toStdString());
    }
    KeePass2Writer writer;
    if (!writer.writeDatabase(&output, &database)) {
        throw std::runtime_error(QString("write KDBX: %1").arg(writer.errorString()).toStdString());
    }
    output.close();
}

void verify_database(const QString& path, const QJsonObject& credentials)
{
    initialize();
    QFile input(path);
    if (!input.open(QIODevice::ReadOnly)) {
        throw std::runtime_error(QString("open generated KDBX: %1").arg(input.errorString()).toStdString());
    }
    Database database;
    KeePass2Reader reader;
    if (!reader.readDatabase(&input, key(credentials), &database)) {
        throw std::runtime_error(QString("reopen generated KDBX: %1").arg(reader.errorString()).toStdString());
    }
    if (database.rootGroup() == nullptr || database.rootGroup()->name().isEmpty()) {
        throw std::runtime_error("generated database has an empty root group");
    }
}

QString datetime(const QDateTime& value)
{
    return value.isValid() ? value.toUTC().toString(Qt::ISODateWithMs) : QString{};
}

QJsonObject serialize_value(const QString& value, bool protect)
{
    return QJsonObject{{"value", value}, {"protected", protect}};
}

QJsonObject serialize_entry(const Entry* entry);

QJsonArray serialize_history(const Entry* entry)
{
    QJsonArray result;
    for (const auto* history : entry->historyItems()) {
        result.append(serialize_entry(history));
    }
    return result;
}

QJsonObject serialize_entry(const Entry* entry)
{
    QJsonObject fields;
    auto field_names = entry->attributes()->customKeys();
    std::sort(field_names.begin(), field_names.end());
    for (const auto& name : field_names) {
        fields.insert(name, serialize_value(entry->attributes()->value(name), entry->attributes()->isProtected(name)));
    }

    QJsonArray attachments;
    auto attachment_names = entry->attachments()->keys();
    std::sort(attachment_names.begin(), attachment_names.end());
    for (const auto& name : attachment_names) {
        attachments.append(QJsonObject{
            {"name", name},
            {"content", QString::fromLatin1(entry->attachments()->value(name).toBase64())},
        });
    }

    QJsonObject custom_data;
    auto custom_names = entry->customData()->keys();
    std::sort(custom_names.begin(), custom_names.end());
    for (const auto& name : custom_names) {
        custom_data.insert(name, serialize_value(entry->customData()->value(name), entry->customData()->isProtected(name)));
    }

    QJsonObject windows;
    for (const auto& association : entry->autoTypeAssociations()->getAll()) {
        windows.insert(association.window, association.sequence);
    }
    const auto& times = entry->timeInfo();
    QJsonObject result{
        {"uuid", entry->uuidToHex()},
        {"title", entry->title()},
        {"username", entry->username()},
        {"password", QJsonObject{{"value", entry->password()}}},
        {"url", entry->url()},
        {"notes", entry->notes()},
        {"tags", QJsonArray::fromStringList(entry->tagList())},
        {"fields", fields},
        {"attachments", attachments},
        {"history", serialize_history(entry)},
        {"created", datetime(times.creationTime())},
        {"modified", datetime(times.lastModificationTime())},
        {"accessed", datetime(times.lastAccessTime())},
        {"recycled", entry->isRecycled()},
        {"icon", entry->iconNumber()},
        {"iconUuid", entry->iconUuid().toString(QUuid::WithoutBraces)},
        {"foreground", entry->foregroundColor()},
        {"background", entry->backgroundColor()},
        {"overrideUrl", entry->overrideUrl()},
        {"autoType", QJsonObject{
            {"enabled", entry->autoTypeEnabled()},
            {"obfuscation", entry->autoTypeObfuscation()},
            {"sequence", entry->defaultAutoTypeSequence()},
            {"windows", windows},
        }},
        {"customData", custom_data},
    };
    if (times.expires()) {
        result.insert("expires", datetime(times.expiryTime()));
    }
    return result;
}

QJsonObject serialize_group(const Group* group, const Metadata* metadata)
{
    QJsonArray groups;
    for (const auto* child : group->children()) {
        groups.append(serialize_group(child, metadata));
    }
    QJsonArray entries;
    for (const auto* entry : group->entries()) {
        entries.append(serialize_entry(entry));
    }
    return QJsonObject{
        {"uuid", group->uuidToHex()},
        {"name", group->name()},
        {"recycleBin", group == metadata->recycleBin()},
        {"templates", group == metadata->entryTemplatesGroup()},
        {"groups", groups},
        {"entries", entries},
    };
}

QByteArray read_database(const QString& path, const QJsonObject& credentials)
{
    initialize();
    QFile input(path);
    if (!input.open(QIODevice::ReadOnly)) {
        throw std::runtime_error(QString("open KDBX input: %1").arg(input.errorString()).toStdString());
    }
    Database database;
    KeePass2Reader reader;
    if (!reader.readDatabase(&input, key(credentials), &database)) {
        throw std::runtime_error(QString("read KDBX: %1").arg(reader.errorString()).toStdString());
    }
    if (database.rootGroup() == nullptr) {
        throw std::runtime_error("KDBX database has no root group");
    }
    return QJsonDocument(QJsonObject{
        {"name", database.metadata()->name()},
        {"root", serialize_group(database.rootGroup(), database.metadata())},
    }).toJson(QJsonDocument::Compact);
}

void set_buffer(bwkp_kpdb_buffer* output, const QByteArray& value)
{
    if (output == nullptr) {
        return;
    }
    output->len = static_cast<size_t>(value.size());
    output->ptr = static_cast<uint8_t*>(std::malloc(output->len));
    if (output->ptr == nullptr && output->len != 0) {
        throw std::bad_alloc();
    }
    if (output->len != 0) {
        std::memcpy(output->ptr, value.constData(), output->len);
    }
}

int fail(bwkp_kpdb_buffer* output, const std::exception& exception)
{
    if (output != nullptr) {
        const auto message = QByteArray::fromStdString(exception.what());
        output->len = static_cast<size_t>(message.size());
        output->ptr = static_cast<uint8_t*>(std::malloc(output->len));
        if (output->ptr != nullptr && output->len != 0) {
            std::memcpy(output->ptr, message.constData(), output->len);
        }
    }
    return 1;
}

} // namespace

extern "C" int32_t bwkp_kpdb_write(const char* path_ptr,
                                     size_t path_len,
                                     const uint8_t* database_ptr,
                                     size_t database_len,
                                     const uint8_t* credentials_ptr,
                                     size_t credentials_len,
                                     const uint8_t* options_ptr,
                                     size_t options_len,
                                     bwkp_kpdb_buffer* error)
{
    try {
        write_database(text(path_ptr, path_len),
                       object(database_ptr, database_len, "database"),
                       object(credentials_ptr, credentials_len, "credentials"),
                       object(options_ptr, options_len, "options"));
        return 0;
    } catch (const std::exception& exception) {
        return fail(error, exception);
    } catch (...) {
        const std::runtime_error exception("unknown KeePassXC bridge failure");
        return fail(error, exception);
    }
}

extern "C" int32_t bwkp_kpdb_verify(const char* path_ptr,
                                      size_t path_len,
                                      const uint8_t* credentials_ptr,
                                      size_t credentials_len,
                                      bwkp_kpdb_buffer* error)
{
    try {
        verify_database(text(path_ptr, path_len), object(credentials_ptr, credentials_len, "credentials"));
        return 0;
    } catch (const std::exception& exception) {
        return fail(error, exception);
    } catch (...) {
        const std::runtime_error exception("unknown KeePassXC bridge failure");
        return fail(error, exception);
    }
}

extern "C" int32_t bwkp_kpdb_read(const char* path_ptr,
                                    size_t path_len,
                                    const uint8_t* credentials_ptr,
                                    size_t credentials_len,
                                    bwkp_kpdb_buffer* output,
                                    bwkp_kpdb_buffer* error)
{
    try {
        set_buffer(output, read_database(text(path_ptr, path_len), object(credentials_ptr, credentials_len, "credentials")));
        return 0;
    } catch (const std::exception& exception) {
        return fail(error, exception);
    } catch (...) {
        const std::runtime_error exception("unknown KeePassXC bridge failure");
        return fail(error, exception);
    }
}

extern "C" const char* bwkp_keepassxc_version(void)
{
    return BWKP_KEEPASSXC_VERSION;
}

extern "C" void bwkp_kpdb_buffer_free(bwkp_kpdb_buffer buffer)
{
    std::free(buffer.ptr);
}
