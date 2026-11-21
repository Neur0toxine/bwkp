# The KDBX-only target has no GUI. Keep the data-model source usable without
# QtGui while leaving upstream GUI builds unchanged.
set(entry_attachments "${SOURCE_DIR}/src/core/EntryAttachments.cpp")
file(READ "${entry_attachments}" source)
set(original_source "${source}")
set(desktop_services_guard "#ifndef BWKP_KDBX_ONLY\n#include <QDesktopServices>\n#endif")
string(FIND "${source}" "${desktop_services_guard}" desktop_services_patched)
if(desktop_services_patched EQUAL -1)
    string(REPLACE
        "#include <QDesktopServices>"
        "${desktop_services_guard}"
        source
        "${source}"
    )
endif()
set(open_attachment_guard "#ifdef BWKP_KDBX_ONLY\n    const bool openOk = false;\n#else\n    const bool openOk = QDesktopServices::openUrl(QUrl::fromLocalFile(m_openedAttachments.value(key)));\n#endif")
string(FIND "${source}" "${open_attachment_guard}" open_attachment_patched)
if(open_attachment_patched EQUAL -1)
    string(REPLACE
        "    const bool openOk = QDesktopServices::openUrl(QUrl::fromLocalFile(m_openedAttachments.value(key)));"
        "${open_attachment_guard}"
        source
        "${source}"
    )
endif()
if(NOT source STREQUAL original_source)
    file(WRITE "${entry_attachments}" "${source}")
endif()

set(metadata "${SOURCE_DIR}/src/core/Metadata.cpp")
file(READ "${metadata}" source)
set(original_source "${source}")
string(REPLACE "#include <QApplication>\n" "" source "${source}")
if(NOT source STREQUAL original_source)
    file(WRITE "${metadata}" "${source}")
endif()

set(kdbx_xml_writer "${SOURCE_DIR}/src/format/KdbxXmlWriter.cpp")
file(READ "${kdbx_xml_writer}" source)
set(original_source "${source}")
set(keeshare_guard "#ifdef WITH_XC_KEESHARE\n#include \"keeshare/KeeShare.h\"\n#include \"keeshare/KeeShareSettings.h\"\n#endif")
string(FIND "${source}" "${keeshare_guard}" keeshare_patched)
if(keeshare_patched EQUAL -1)
    string(REPLACE
        "#include \"keeshare/KeeShare.h\"\n#include \"keeshare/KeeShareSettings.h\""
        "${keeshare_guard}"
        source
        "${source}"
    )
endif()
if(NOT source STREQUAL original_source)
    file(WRITE "${kdbx_xml_writer}" "${source}")
endif()

function(replace_source file original replacement)
    file(READ "${file}" source)
    string(FIND "${source}" "${replacement}" patched)
    if(NOT patched EQUAL -1)
        return()
    endif()
    string(FIND "${source}" "${original}" found)
    if(found EQUAL -1)
        message(FATAL_ERROR "Cannot apply the bwkp compatibility patch to ${file}")
    endif()
    string(REPLACE "${original}" "${replacement}" source "${source}")
    file(WRITE "${file}" "${source}")
endfunction()

# The reduced build uses Qt 5 on the supported release platforms.
set(xml_reader "${SOURCE_DIR}/src/format/KdbxXmlReader.cpp")
replace_source("${xml_reader}" "file.open(QIODeviceBase::ReadOnly)" "file.open(QIODevice::ReadOnly)")
replace_source("${xml_reader}"
    [=[%" PRIdQSIZETYPE " invalid group reference(s)]=] "%d invalid group reference(s)")
replace_source("${xml_reader}"
    [=[%" PRIdQSIZETYPE " invalid entry reference(s)]=] "%d invalid entry reference(s)")

set(entry_placeholders "${SOURCE_DIR}/src/core/EntryPlaceholders.cpp")
replace_source("${entry_placeholders}" "#include <QDateTime>" "#include <QDateTime>\n#include <QDebug>")

# The hardware drivers are outside the KDBX-only target.
set(challenge_key "${SOURCE_DIR}/src/keys/ChallengeResponseKey.cpp")
set(challenge_start [=[    m_error.clear();
    auto result =]=])
set(challenge_replacement [=[#ifdef BWKP_KDBX_ONLY
    Q_UNUSED(challenge)
    m_key.clear();
    m_error = QStringLiteral("Hardware keys are unavailable");
    return false;
#else
    m_error.clear();
    auto result =]=])
replace_source("${challenge_key}" "${challenge_start}" "${challenge_replacement}")
replace_source("${challenge_key}"
    "    return result == YubiKey::ChallengeResult::YCR_SUCCESS;"
    "    return result == YubiKey::ChallengeResult::YCR_SUCCESS;\n#endif")

# Botan's Argon2 implements version 0x13. Retain KeePassXC 2.7's version-aware
# derivation for legacy databases and use the 2.8 implementation for 0x13.
set(argon2_kdf "${SOURCE_DIR}/src/crypto/kdf/Argon2Kdf.cpp")
replace_source("${argon2_kdf}" "#include <botan/pwdhash.h>"
    "#include <botan/pwdhash.h>\n#ifdef BWKP_KDBX_ONLY\n#include <argon2.h>\n#endif")
set(argon2_start "    const std::string algoName =")
set(argon2_replacement [=[#ifdef BWKP_KDBX_ONLY
    if (version() != ARGON2_VERSION_13) {
        const int rc = argon2_hash(rounds(),
                                   memory(),
                                   parallelism(),
                                   raw.constData(),
                                   raw.size(),
                                   seed().constData(),
                                   seed().size(),
                                   result.data(),
                                   result.size(),
                                   nullptr,
                                   0,
                                   type() == Type::Argon2d ? Argon2_d : Argon2_id,
                                   version());
        if (rc != ARGON2_OK) {
            qWarning("Argon2 error: %s", argon2_error_message(rc));
            return false;
        }
        return true;
    }
#endif

    const std::string algoName =]=])
replace_source("${argon2_kdf}" "${argon2_start}" "${argon2_replacement}")

if(NOT ANDROID_BUILD)
    return()
endif()

set(file_watcher "${SOURCE_DIR}/src/core/FileWatcher.cpp")
file(READ "${file_watcher}" source)

set(original [=[
    bool forcePolling = false;
    const auto NFS_SUPER_MAGIC = 0x6969;
]=])
set(replacement [=[
    bool forcePolling = false;
#ifndef NFS_SUPER_MAGIC
    const auto NFS_SUPER_MAGIC = 0x6969;
#endif
]=])

string(FIND "${source}" "${replacement}" file_watcher_patched)
if(file_watcher_patched EQUAL -1)
    string(FIND "${source}" "${original}" patch_location)
    if(patch_location EQUAL -1)
        message(FATAL_ERROR "KeePassXC FileWatcher.cpp no longer matches the expected 2.8.0 source")
    endif()

    string(REPLACE "${original}" "${replacement}" source "${source}")
    file(WRITE "${file_watcher}" "${source}")
endif()
