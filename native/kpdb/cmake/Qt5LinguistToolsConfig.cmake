# KeePassXC requires Qt's translation tool at configure time even when the
# translation targets are not built. The exporter only builds keepassx_core.
set(Qt5LinguistTools_FOUND TRUE)
function(qt5_add_translation output_variable)
    set(${output_variable} "" PARENT_SCOPE)
endfunction()
