// V0003 — Vector indexes
// NOTE: Adjust vector.dimensions to match your embedding model.

// Node vector index for MemoryNode.embedding
CREATE VECTOR INDEX memNodeEmbedding IF NOT EXISTS
FOR (n:MemoryNode)
ON n.embedding
OPTIONS { indexConfig: {
  `vector.dimensions`: 1536,
  `vector.similarity_function`: 'cosine'
}};

// Optional: Observation embedding index (if you embed raw chunks)
CREATE VECTOR INDEX observationEmbedding IF NOT EXISTS
FOR (o:Observation)
ON o.embedding
OPTIONS { indexConfig: {
  `vector.dimensions`: 1536,
  `vector.similarity_function`: 'cosine'
}};

MERGE (m:Migration {version: 3})
ON CREATE SET m.name='V0003__vector_indexes',
              m.applied_at=datetime(),
              m.checksum=null;

MATCH (s:SchemaMeta {key:'schema'})
WITH s
WHERE coalesce(s.current_version,0) < 3
SET s.current_version = 3,
    s.updated_at = datetime();
