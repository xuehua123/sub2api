//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWSPassthroughUsageMeta_InitFromFirstFrame_MappedModelCandidate(t *testing.T) {
	body := []byte(`{"type":"response.create","model":"sol","reasoning":{"effort":"max"}}`)

	meta := newOpenAIWSPassthroughUsageMeta("sol", body)
	meta.initFromFirstFrame(body, "gpt-5.6-sol")

	got := meta.reasoningEffort.Load()
	require.NotNil(t, got, "reasoning effort should be set")
	require.Equal(t, "max", *got, "mapped model gpt-5.6-sol should preserve max")
}

func TestWSPassthroughUsageMeta_InitFromFirstFrame_NonGPT56FallsBackToXHigh(t *testing.T) {
	body := []byte(`{"type":"response.create","model":"gpt-5.4","reasoning":{"effort":"max"}}`)

	meta := newOpenAIWSPassthroughUsageMeta("gpt-5.4", body)
	meta.initFromFirstFrame(body, "gpt-5.4")
	meta.captureRequestedReasoningEffort(body, "gpt-5.4")

	got := meta.reasoningEffort.Load()
	require.NotNil(t, got)
	require.Equal(t, "xhigh", *got, "non-5.6 model should normalize max to xhigh")
	requested := meta.requestedReasoningEffort.Load()
	require.NotNil(t, requested)
	require.Equal(t, "max", *requested, "usage should keep the pre-mapping requested effort")
}

func TestWSPassthroughUsageMeta_UpdateFromResponseCreate_MappedModelCandidate(t *testing.T) {
	body := []byte(`{"type":"response.create","model":"sol","reasoning":{"effort":"max"}}`)

	meta := newOpenAIWSPassthroughUsageMeta("sol", body)
	meta.updateFromResponseCreate(body, "gpt-5.6-sol", "sol")

	got := meta.reasoningEffort.Load()
	require.NotNil(t, got)
	require.Equal(t, "max", *got, "mapped model should preserve max on multi-turn update")
}

func TestWSPassthroughUsageMeta_CurrentFrameSuffixOverridesSessionFallback(t *testing.T) {
	meta := newOpenAIWSPassthroughUsageMeta("gpt-5.6-low", []byte(`{"type":"response.create","model":"gpt-5.6-low"}`))

	currentFrame := []byte(`{"type":"response.create","model":"gpt-5.6-high"}`)
	meta.captureRequestedReasoningEffort(currentFrame)
	meta.updateFromResponseCreate(currentFrame, "upstream-gpt-5.6-high", "gpt-5.6-high")

	requested := meta.requestedReasoningEffort.Load()
	require.NotNil(t, requested)
	require.Equal(t, "high", *requested)
	require.Equal(t, "gpt-5.6-high", meta.failureModel("gpt-5.6-low"), "failure attribution must use the accepted current turn upstream model")
}
