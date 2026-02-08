// Phase 75B: Convert conversation GENERALIZES edges to THEME_OF
// GENERALIZES between conversation_observation and conversation_theme has different
// semantics than GENERALIZES between L0 code nodes and L1 hidden patterns.
// THEME_OF explicitly captures the "this observation belongs to this theme" relationship.

// Copy properties to new THEME_OF edges
CALL {
  MATCH (o:MemoryNode {role_type: 'conversation_observation'})-[r:GENERALIZES]->(t:MemoryNode {role_type: 'conversation_theme'})
  CREATE (o)-[n:THEME_OF]->(t)
  SET n = properties(r)
  DELETE r
} IN TRANSACTIONS OF 500 ROWS;
