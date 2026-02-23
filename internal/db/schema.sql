-- Sessions table
CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    project     TEXT NOT NULL,
    machine     TEXT NOT NULL DEFAULT 'local',
    agent       TEXT NOT NULL DEFAULT 'claude',
    first_message TEXT,
    started_at  TEXT,
    ended_at    TEXT,
    message_count INTEGER NOT NULL DEFAULT 0,
    file_path   TEXT,
    file_size   INTEGER,
    file_mtime  INTEGER,
    file_hash   TEXT,
    parent_session_id TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Messages table with ordinal for efficient range queries
CREATE TABLE IF NOT EXISTS messages (
    id             INTEGER PRIMARY KEY,
    session_id     TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    ordinal        INTEGER NOT NULL,
    role           TEXT NOT NULL,
    content        TEXT NOT NULL,
    timestamp      TEXT,
    has_thinking   INTEGER NOT NULL DEFAULT 0,
    has_tool_use   INTEGER NOT NULL DEFAULT 0,
    content_length INTEGER NOT NULL DEFAULT 0,
    UNIQUE(session_id, ordinal)
);

-- Stats table maintained by triggers
CREATE TABLE IF NOT EXISTS stats (
    key   TEXT PRIMARY KEY,
    value INTEGER NOT NULL DEFAULT 0
);

INSERT OR IGNORE INTO stats (key, value) VALUES ('session_count', 0);
INSERT OR IGNORE INTO stats (key, value) VALUES ('message_count', 0);

-- Triggers for stats maintenance
CREATE TRIGGER IF NOT EXISTS sessions_insert_stats AFTER INSERT ON sessions BEGIN
    UPDATE stats SET value = value + 1 WHERE key = 'session_count';
END;

CREATE TRIGGER IF NOT EXISTS sessions_delete_stats AFTER DELETE ON sessions BEGIN
    UPDATE stats SET value = value - 1 WHERE key = 'session_count';
END;

CREATE TRIGGER IF NOT EXISTS messages_insert_stats AFTER INSERT ON messages BEGIN
    UPDATE stats SET value = value + 1 WHERE key = 'message_count';
END;

CREATE TRIGGER IF NOT EXISTS messages_delete_stats AFTER DELETE ON messages BEGIN
    UPDATE stats SET value = value - 1 WHERE key = 'message_count';
END;

-- Indexes
CREATE INDEX IF NOT EXISTS idx_sessions_ended
    ON sessions(ended_at DESC, id);
CREATE INDEX IF NOT EXISTS idx_sessions_project
    ON sessions(project);
CREATE INDEX IF NOT EXISTS idx_sessions_machine
    ON sessions(machine);
CREATE INDEX IF NOT EXISTS idx_messages_session_ordinal
    ON messages(session_id, ordinal);
CREATE INDEX IF NOT EXISTS idx_messages_session_role
    ON messages(session_id, role);

CREATE INDEX IF NOT EXISTS idx_sessions_parent
    ON sessions(parent_session_id)
    WHERE parent_session_id IS NOT NULL;

-- Analytics indexes
CREATE INDEX IF NOT EXISTS idx_sessions_started
    ON sessions(started_at);
CREATE INDEX IF NOT EXISTS idx_sessions_message_count
    ON sessions(message_count);
CREATE INDEX IF NOT EXISTS idx_sessions_agent
    ON sessions(agent);

-- Tool calls table
CREATE TABLE IF NOT EXISTS tool_calls (
    id         INTEGER PRIMARY KEY,
    message_id INTEGER NOT NULL
        REFERENCES messages(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL
        REFERENCES sessions(id) ON DELETE CASCADE,
    tool_name  TEXT NOT NULL,
    category   TEXT NOT NULL,
    tool_use_id TEXT,
    input_json  TEXT,
    skill_name  TEXT,
    result_content_length INTEGER
);

CREATE INDEX IF NOT EXISTS idx_tool_calls_session
    ON tool_calls(session_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_category
    ON tool_calls(category);
CREATE INDEX IF NOT EXISTS idx_tool_calls_skill
    ON tool_calls(skill_name)
    WHERE skill_name IS NOT NULL;

-- Insights table for AI-generated activity insights
CREATE TABLE IF NOT EXISTS insights (
    id          INTEGER PRIMARY KEY,
    type        TEXT NOT NULL,
    date_from   TEXT NOT NULL,
    date_to     TEXT NOT NULL,
    project     TEXT,
    agent       TEXT NOT NULL,
    model       TEXT,
    prompt      TEXT,
    content     TEXT NOT NULL,
    created_at  TEXT NOT NULL
        DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_insights_lookup
    ON insights(type, date_from, project);

CREATE INDEX IF NOT EXISTS idx_insights_created
    ON insights(created_at DESC);
