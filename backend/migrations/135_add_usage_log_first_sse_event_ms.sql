-- Add first upstream SSE event latency to usage logs.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS first_sse_event_ms INTEGER;
