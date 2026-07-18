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

file(WRITE "${entry_view}" "${source}")
