(call_expression
  function: [
    (identifier) @callee
    (qualified_identifier
      name: (identifier) @callee)
    (field_expression
      field: (field_identifier) @callee)
  ]) @caller
