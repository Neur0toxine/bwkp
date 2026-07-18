foreach(cmake_file IN ITEMS
        "${SOURCE_DIR}/CMakeLists.txt"
        "${SOURCE_DIR}/src/CMakeLists.txt")
    file(READ "${cmake_file}" source)
    set(original_source "${source}")
    string(REPLACE "\${CMAKE_SOURCE_DIR}" "${SOURCE_DIR}" source "${source}")
    if(NOT source STREQUAL original_source)
        file(WRITE "${cmake_file}" "${source}")
    endif()
endforeach()

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
        message(FATAL_ERROR "KeePassXC FileWatcher.cpp no longer matches the expected 2.7.12 source")
    endif()

    string(REPLACE "${original}" "${replacement}" source "${source}")
    file(WRITE "${file_watcher}" "${source}")
endif()

set(entry_view "${SOURCE_DIR}/src/gui/entry/EntryView.cpp")
file(READ "${entry_view}" source)
set(original_source "${source}")

string(REPLACE
    "#include <QAccessible>\n#include <QDrag>"
    "#include <QCoreApplication>\n#include <QDrag>\n#include <QKeyEvent>"
    source
    "${source}"
)
string(REPLACE
    "        QAccessibleEvent accessibleEvent(this, QAccessible::PageChanged);\n"
    ""
    source
    "${source}"
)
string(REPLACE
    "            QAccessible::updateAccessibility(&accessibleEvent);\n"
    ""
    source
    "${source}"
)

string(FIND "${source}" "#include <QKeyEvent>" entry_view_patched)
if(entry_view_patched EQUAL -1)
    message(FATAL_ERROR "KeePassXC EntryView.cpp no longer matches the expected 2.7.12 source")
endif()

if(NOT source STREQUAL original_source)
    file(WRITE "${entry_view}" "${source}")
endif()
