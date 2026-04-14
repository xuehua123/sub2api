-- Migration: 103_add_user_default_chat_api_key_id
-- Persist each user's preferred LobeHub chat API key selection.

ALTER TABLE users ADD COLUMN IF NOT EXISTS default_chat_api_key_id BIGINT NULL;
CREATE INDEX IF NOT EXISTS idx_users_default_chat_api_key_id ON users(default_chat_api_key_id);
