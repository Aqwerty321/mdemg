(call_expression
  function: [
    (identifier) @callee
    (scoped_identifier
      name: (identifier) @callee)
    (field_expression
      field: (field_identifier) @callee)
  ]) @caller
