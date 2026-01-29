// Cypher Parser Test Fixture
// Tests symbol extraction for Neo4j graph database
// Line numbers are predictable for UPTS validation

// === Pattern: Node label constraints ===
// Line 6-8
CREATE CONSTRAINT user_id IF NOT EXISTS FOR (u:User) REQUIRE u.id IS UNIQUE;
CREATE CONSTRAINT space_id IF NOT EXISTS FOR (s:Space) REQUIRE s.id IS UNIQUE;
CREATE CONSTRAINT file_path IF NOT EXISTS FOR (f:File) REQUIRE (f.space_id, f.path) IS UNIQUE;

// === Pattern: Indexes ===
// Line 11-14
CREATE INDEX user_email IF NOT EXISTS FOR (u:User) ON (u.email);
CREATE INDEX file_language IF NOT EXISTS FOR (f:File) ON (f.language);
CREATE INDEX symbol_name IF NOT EXISTS FOR (s:Symbol) ON (s.name);
CREATE INDEX symbol_type IF NOT EXISTS FOR (s:Symbol) ON (s.type);

// === Pattern: Node creation ===
// Line 17-23
CREATE (u:User {
    id: randomUUID(),
    name: 'Test User',
    email: 'test@example.com',
    status: 'active',
    created_at: datetime()
});

// Line 25-32
CREATE (s:Space {
    id: randomUUID(),
    name: 'Test Space',
    description: 'A test workspace',
    owner_id: $user_id,
    visibility: 'private',
    created_at: datetime()
});

// === Pattern: Relationship creation ===
// Line 35-36
MATCH (u:User {id: $user_id}), (s:Space {id: $space_id})
CREATE (u)-[:OWNS {since: datetime()}]->(s);

// Line 38-39
MATCH (f:File {id: $file_id}), (s:Symbol {id: $symbol_id})
CREATE (f)-[:DEFINES_SYMBOL {line: $line, exported: true}]->(s);

// === Pattern: Complex queries ===
// Line 42-50
MATCH (f:File)-[:DEFINES_SYMBOL]->(s:Symbol)
WHERE f.space_id = $space_id
  AND s.type IN ['function', 'class', 'method']
  AND s.exported = true
RETURN f.path AS file_path,
       s.name AS symbol_name,
       s.type AS symbol_type,
       s.line AS line_number
ORDER BY f.path, s.line;

// === Pattern: Aggregation queries ===
// Line 53-60
MATCH (s:Space {id: $space_id})-[:CONTAINS]->(f:File)
OPTIONAL MATCH (f)-[:DEFINES_SYMBOL]->(sym:Symbol)
WITH s, f, count(sym) AS symbol_count
RETURN s.name AS space_name,
       count(f) AS file_count,
       sum(symbol_count) AS total_symbols
GROUP BY s.name;

// === Pattern: Graph traversal ===
// Line 63-70
MATCH path = (caller:Symbol)-[:CALLS*1..3]->(callee:Symbol)
WHERE caller.id = $symbol_id
WITH caller, callee, length(path) AS depth
RETURN DISTINCT 
       caller.name AS caller_name,
       callee.name AS callee_name,
       depth
ORDER BY depth, callee.name;

// === Pattern: MDEMG-specific labels ===
// Line 73-76
CREATE (chunk:Chunk {
    id: randomUUID(),
    content: $content,
    embedding: $embedding_vector,
    file_id: $file_id,
    start_line: $start_line,
    end_line: $end_line
});

// Line 78-79
MATCH (chunk:Chunk {id: $chunk_id}), (sym:Symbol {id: $symbol_id})
CREATE (chunk)-[:REFERENCES {confidence: 0.95}]->(sym);
