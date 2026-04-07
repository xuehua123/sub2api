ALTER TABLE users
  ADD COLUMN IF NOT EXISTS default_chat_api_key_id BIGINT DEFAULT NULL;

COMMENT ON COLUMN users.default_chat_api_key_id IS '用户默认用于 LobeHub 直开的 API Key ID';
