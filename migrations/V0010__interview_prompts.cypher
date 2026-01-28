// V0010: Interview Prompt Schema
// Support for the Gap Interview APE job

// Create constraint for unique prompt IDs
CREATE CONSTRAINT interview_prompt_id IF NOT EXISTS
FOR (p:InterviewPrompt) REQUIRE p.prompt_id IS UNIQUE;

// Create indexes for efficient querying
CREATE INDEX interview_prompt_status IF NOT EXISTS
FOR (p:InterviewPrompt) ON (p.status);

CREATE INDEX interview_prompt_gap_id IF NOT EXISTS
FOR (p:InterviewPrompt) ON (p.gap_id);

CREATE INDEX interview_prompt_priority IF NOT EXISTS
FOR (p:InterviewPrompt) ON (p.priority);

CREATE INDEX interview_prompt_space IF NOT EXISTS
FOR (p:InterviewPrompt) ON (p.space_id);

// Composite index for common query pattern (pending prompts by priority)
CREATE INDEX interview_prompt_status_priority IF NOT EXISTS
FOR (p:InterviewPrompt) ON (p.status, p.priority);

// Record migration
MERGE (m:Migration {version: 10})
ON CREATE SET m.name='V0010__interview_prompts',
              m.applied_at=datetime(),
              m.checksum=null;

// Update schema version
MATCH (s:SchemaMeta {key:'schema'})
WITH s
WHERE coalesce(s.current_version, 0) < 10
SET s.current_version = 10,
    s.updated_at = datetime();
