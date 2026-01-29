-- SQL Parser Test Fixture
-- Tests symbol extraction for database schemas
-- Line numbers are predictable for UPTS validation

-- === Pattern: Table definitions ===
-- Line 7-15
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Line 17-24
CREATE TABLE items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    value INTEGER DEFAULT 0,
    priority VARCHAR(20) DEFAULT 'medium',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- === Pattern: Indexes ===
-- Line 27-29
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_status ON users(status);
CREATE INDEX idx_items_user_id ON items(user_id);

-- === Pattern: Views ===
-- Line 32-39
CREATE VIEW active_users AS
SELECT 
    id,
    name,
    email,
    created_at
FROM users
WHERE status = 'active';

-- Line 41-49
CREATE VIEW user_item_summary AS
SELECT 
    u.id AS user_id,
    u.name AS user_name,
    COUNT(i.id) AS item_count,
    COALESCE(SUM(i.value), 0) AS total_value
FROM users u
LEFT JOIN items i ON u.id = i.user_id
GROUP BY u.id, u.name;

-- === Pattern: Functions/Procedures ===
-- Line 52-58
CREATE OR REPLACE FUNCTION update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Line 60-68
CREATE OR REPLACE FUNCTION get_user_items(p_user_id UUID)
RETURNS TABLE (
    item_id UUID,
    item_name VARCHAR,
    item_value INTEGER
) AS $$
BEGIN
    RETURN QUERY SELECT id, name, value FROM items WHERE user_id = p_user_id;
END;
$$ LANGUAGE plpgsql;

-- === Pattern: Triggers ===
-- Line 71-74
CREATE TRIGGER update_users_timestamp
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_timestamp();

-- === Pattern: Constraints ===
-- Line 77-78
ALTER TABLE items ADD CONSTRAINT check_value_positive CHECK (value >= 0);
ALTER TABLE users ADD CONSTRAINT check_status_valid CHECK (status IN ('active', 'inactive', 'pending'));

-- === Pattern: Enum type (PostgreSQL) ===
-- Line 81
CREATE TYPE priority_level AS ENUM ('low', 'medium', 'high', 'critical');

-- === Pattern: Sequences ===
-- Line 84
CREATE SEQUENCE order_number_seq START 1000 INCREMENT 1;
