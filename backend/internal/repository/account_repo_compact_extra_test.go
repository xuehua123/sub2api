package repository

import "testing"

func TestShouldEnqueueSchedulerOutboxForExtraUpdates_CompactCapabilityKeysAreRelevant(t *testing.T) {
	updates := map[string]any{
		"openai_compact_supported":  true,
		"openai_compact_checked_at": "2026-04-10T10:00:00Z",
	}

	if !shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
		t.Fatalf("expected compact capability updates to enqueue scheduler outbox")
	}
}

func TestShouldEnqueueSchedulerOutboxForExtraUpdates_OpenAIResponsesCapabilityKeysAreRelevant(t *testing.T) {
	updates := map[string]any{
		"openai_responses_mode":      "force_chat_completions",
		"openai_responses_supported": false,
	}

	if !shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
		t.Fatalf("expected responses capability updates to enqueue scheduler outbox")
	}
}

func TestShouldEnqueueSchedulerOutboxForExtraUpdates_AccountHealthProbeKeysAreObservational(t *testing.T) {
	updates := map[string]any{
		"ops_health_probe_status":     "success",
		"ops_health_probe_checked_at": "2026-06-06T10:00:00Z",
		"ops_health_probe_latency_ms": 1234,
		"ops_health_probe_model_id":   "gpt-5.4-mini",
		"ops_health_probe_error":      "",
	}

	if shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
		t.Fatalf("expected account health probe updates to skip scheduler outbox")
	}
}

func TestShouldEnqueueSchedulerOutboxForExtraUpdates_TagsAreObservational(t *testing.T) {
	updates := map[string]any{
		"tags": []string{"pro", "生图"},
	}

	if shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
		t.Fatalf("expected account tags updates to skip scheduler outbox")
	}
}
