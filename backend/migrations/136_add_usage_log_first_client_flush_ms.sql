ALTER TABLE usage_logs
  ADD COLUMN IF NOT EXISTS first_client_flush_ms INTEGER;
