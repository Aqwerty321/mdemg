(use_declaration
  argument: [
    (scoped_identifier) @import_path
    (identifier) @import_path
    (use_wildcard) @import_path
    (use_list) @import_path
    (scoped_use_list
      path: (scoped_identifier) @import_path)
  ]) @source_file
