SET LOCAL lock_timeout = '5s';
CREATE TABLE IF NOT EXISTS upstream_group_annotations (
    connection_id BIGINT NOT NULL REFERENCES upstream_connections(id) ON DELETE CASCADE,
    remote_key TEXT NOT NULL,
    tags TEXT[] NOT NULL DEFAULT '{}',
    favorite BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (connection_id, remote_key)
);
CREATE INDEX IF NOT EXISTS upstream_group_annotations_tags ON upstream_group_annotations USING GIN (tags);
