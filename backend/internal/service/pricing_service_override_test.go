package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// gpt55OverrideCatalogJSON 镜像真实目录形态：长上下文以 above_272k 绝对价字段表达。
const gpt55OverrideCatalogJSON = `{
	"gpt-5.5": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 5e-06, "input_cost_per_token_priority": 1.25e-05,
		"output_cost_per_token": 3e-05, "output_cost_per_token_priority": 7.5e-05,
		"cache_read_input_token_cost": 5e-07,
		"input_cost_per_token_above_272k_tokens": 1e-05,
		"output_cost_per_token_above_272k_tokens": 4.5e-05,
		"cache_read_input_token_cost_above_272k_tokens": 1e-06},
	"gpt-5.4": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 2.5e-06, "output_cost_per_token": 1.5e-05,
		"cache_read_input_token_cost": 2.5e-07,
		"input_cost_per_token_above_272k_tokens": 5e-06,
		"output_cost_per_token_above_272k_tokens": 2.25e-05}
}`

func newPricingServiceWithOverride(t *testing.T, overrideJSON string) *PricingService {
	t.Helper()
	path := filepath.Join(t.TempDir(), "overrides.json")
	require.NoError(t, os.WriteFile(path, []byte(overrideJSON), 0644))
	svc := &PricingService{cfg: &config.Config{}}
	svc.cfg.Pricing.OverrideFile = path
	return svc
}

// override 的旗舰用例：显式 threshold=0 压住 above 折算，把目录条目的阶梯关成标准价。
func TestPricingOverride_ExplicitZeroThresholdDisablesCatalogLadder(t *testing.T) {
	svc := newPricingServiceWithOverride(t, `{"gpt-5.5": {"long_context_input_token_threshold": 0}}`)
	data, err := svc.parsePricingData([]byte(gpt55OverrideCatalogJSON))
	require.NoError(t, err)

	patched := data["gpt-5.5"]
	require.NotNil(t, patched)
	require.Zero(t, patched.LongContextInputTokenThreshold)
	require.Zero(t, patched.LongContextInputCostMultiplier)
	require.Empty(t, patched.ContextPriceTiers, "关闭阶梯后不得保留价格页的 raw tier 来源")
	require.Zero(t, patched.InputCostPerTokenAbove272K)
	require.Zero(t, patched.OutputCostPerTokenAbove272K)
	require.Zero(t, patched.CacheReadInputTokenCostAbove272K)
	require.InDelta(t, 5e-6, patched.InputCostPerToken, 1e-12, "补丁不得影响基础价")
	require.InDelta(t, 3e-5, patched.OutputCostPerToken, 1e-12)
	require.Equal(t, 272000, data["gpt-5.4"].LongContextInputTokenThreshold, "未覆盖的模型保持目录阶梯")

	svc.pricingData = data
	billing := NewBillingService(&config.Config{}, svc)
	tokens := UsageTokens{InputTokens: 300000, OutputTokens: 1000, CacheReadTokens: 10000}
	cost, err := billing.CalculateCost("gpt-5.5", tokens, 1)
	require.NoError(t, err)
	require.False(t, cost.LongContextBillingApplied)
	require.InDelta(t, 300000*5e-6, cost.InputCost, 1e-10)
	require.InDelta(t, 1000*3e-5, cost.OutputCost, 1e-10)
	require.InDelta(t, 10000*5e-7, cost.CacheReadCost, 1e-10)
}

func TestPricingOverride_ExplicitCacheReadMultiplierOverridesDerivedValue(t *testing.T) {
	svc := newPricingServiceWithOverride(t, `{"gpt-5.5": {"long_context_cache_read_cost_multiplier": 3}}`)
	data, err := svc.parsePricingData([]byte(gpt55OverrideCatalogJSON))
	require.NoError(t, err)

	patched := data["gpt-5.5"]
	require.NotNil(t, patched)
	require.Equal(t, 272000, patched.LongContextInputTokenThreshold, "cache-read 补丁不得阻止主阶梯从 above 价推导")
	require.InDelta(t, 2, patched.LongContextInputCostMultiplier, 1e-12)
	require.InDelta(t, 1.5, patched.LongContextOutputCostMultiplier, 1e-12)
	require.InDelta(t, 3, patched.LongContextCacheReadCostMultiplier, 1e-12)
}

func TestPricingOverride_FieldLevelMergeKeepsOtherFields(t *testing.T) {
	svc := newPricingServiceWithOverride(t, `{"gpt-5.4": {"input_cost_per_token": 3e-06}}`)
	data, err := svc.parsePricingData([]byte(gpt55OverrideCatalogJSON))
	require.NoError(t, err)

	patched := data["gpt-5.4"]
	require.InDelta(t, 3e-6, patched.InputCostPerToken, 1e-12)
	require.InDelta(t, 1.5e-5, patched.OutputCostPerToken, 1e-12, "未覆盖字段保持目录值")
	require.Equal(t, "openai", patched.LiteLLMProvider)
	require.Equal(t, 272000, patched.LongContextInputTokenThreshold, "above 折算仍生效")
	// 折算发生在合并之后：above 价不变、基础价被补丁改小，倍率随之变化。
	require.InDelta(t, 5.0/3.0, patched.LongContextInputCostMultiplier, 1e-9)
}

func TestPricingOverride_NullFieldValueRemovesField(t *testing.T) {
	svc := newPricingServiceWithOverride(t, `{"gpt-5.5": {
		"input_cost_per_token_above_272k_tokens": null,
		"output_cost_per_token_above_272k_tokens": null,
		"cache_read_input_token_cost_above_272k_tokens": null}}`)
	data, err := svc.parsePricingData([]byte(gpt55OverrideCatalogJSON))
	require.NoError(t, err)
	require.Zero(t, data["gpt-5.5"].LongContextInputTokenThreshold, "above 字段删除后不再折算阶梯")
	require.InDelta(t, 5e-6, data["gpt-5.5"].InputCostPerToken, 1e-12)
}

// 完整加载管线：纯补丁不得抢在回退合并前建条目（否则回退完整条目被跳过、
// 其余分项价变 0 少收）；目录/回退都没有的模型作为独立条目并入。
func TestPricingOverride_LoadPipelineAddsNewModelAndPatchesFallbackOnly(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")
	require.NoError(t, os.WriteFile(catalogPath, []byte(`{
		"remote-model": {"litellm_provider": "test", "mode": "chat",
			"input_cost_per_token": 1e-06, "output_cost_per_token": 2e-06}
	}`), 0644))
	fallbackPath := filepath.Join(dir, "fallback.json")
	require.NoError(t, os.WriteFile(fallbackPath, []byte(`{
		"fallback-only-model": {"litellm_provider": "test", "mode": "chat",
			"input_cost_per_token": 4e-06, "output_cost_per_token": 8e-06,
			"cache_read_input_token_cost": 4e-07}
	}`), 0644))
	overridePath := filepath.Join(dir, "overrides.json")
	require.NoError(t, os.WriteFile(overridePath, []byte(`{
		"fallback-only-model": {"input_cost_per_token": 9e-06},
		"override-new-model": {"litellm_provider": "test", "mode": "chat",
			"input_cost_per_token": 5e-06, "output_cost_per_token": 1e-05}
	}`), 0644))

	svc := &PricingService{cfg: &config.Config{}}
	svc.cfg.Pricing.FallbackFile = fallbackPath
	svc.cfg.Pricing.OverrideFile = overridePath
	require.NoError(t, svc.loadPricingData(catalogPath))

	patched := svc.pricingData["fallback-only-model"]
	require.NotNil(t, patched)
	require.InDelta(t, 9e-6, patched.InputCostPerToken, 1e-12)
	require.InDelta(t, 8e-6, patched.OutputCostPerToken, 1e-12, "回退条目的其余字段必须保留")
	require.InDelta(t, 4e-7, patched.CacheReadInputTokenCost, 1e-12)

	added := svc.pricingData["override-new-model"]
	require.NotNil(t, added)
	require.InDelta(t, 5e-6, added.InputCostPerToken, 1e-12)
	require.InDelta(t, 1e-5, added.OutputCostPerToken, 1e-12)

	require.InDelta(t, 1e-6, svc.pricingData["remote-model"].InputCostPerToken, 1e-12)
}

// 拼错模型名（或纯补丁落在不存在的模型上）会被有效性过滤丢弃，必须有哨兵 WARN。
func TestPricingOverride_IneffectiveEntryWarns(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()

	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")
	require.NoError(t, os.WriteFile(catalogPath, []byte(`{
		"remote-model": {"litellm_provider": "test", "mode": "chat", "input_cost_per_token": 1e-06}
	}`), 0644))
	overridePath := filepath.Join(dir, "overrides.json")
	require.NoError(t, os.WriteFile(overridePath, []byte(`{
		"typo-model": {"long_context_input_token_threshold": 0}
	}`), 0644))

	svc := &PricingService{cfg: &config.Config{}}
	svc.cfg.Pricing.OverrideFile = overridePath
	require.NoError(t, svc.loadPricingData(catalogPath))

	require.NotContains(t, svc.pricingData, "typo-model")
	require.True(t, logSink.ContainsMessageAtLevel("override had no effect for 1 model(s): typo-model", "warn"))
}

func TestPricingOverride_NonObjectEntryKeepsCatalogEntry(t *testing.T) {
	svc := newPricingServiceWithOverride(t, `{"gpt-5.5": "oops"}`)
	data, err := svc.parsePricingData([]byte(gpt55OverrideCatalogJSON))
	require.NoError(t, err)
	require.Equal(t, 272000, data["gpt-5.5"].LongContextInputTokenThreshold, "非法补丁忽略，目录条目原样保留")
	require.InDelta(t, 5e-6, data["gpt-5.5"].InputCostPerToken, 1e-12)
}

func TestPricingOverride_NullCatalogEntryCanBeReplaced(t *testing.T) {
	svc := newPricingServiceWithOverride(t, `{"patched-model": {
		"litellm_provider": "test", "mode": "chat",
		"input_cost_per_token": 2e-06, "output_cost_per_token": 4e-06}}`)

	var (
		data map[string]*LiteLLMModelPricing
		err  error
	)
	require.NotPanics(t, func() {
		data, err = svc.parsePricingData([]byte(`{"patched-model": null}`))
	})
	require.NoError(t, err)
	require.Contains(t, data, "patched-model")
	require.InDelta(t, 2e-6, data["patched-model"].InputCostPerToken, 1e-12)
	require.InDelta(t, 4e-6, data["patched-model"].OutputCostPerToken, 1e-12)
}

func TestPricingOverride_MissingOrInvalidFileIsIgnored(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		svc := &PricingService{cfg: &config.Config{}}
		svc.cfg.Pricing.OverrideFile = filepath.Join(t.TempDir(), "absent.json")
		data, err := svc.parsePricingData([]byte(gpt55OverrideCatalogJSON))
		require.NoError(t, err)
		require.Equal(t, 272000, data["gpt-5.5"].LongContextInputTokenThreshold)
	})

	t.Run("invalid json", func(t *testing.T) {
		svc := newPricingServiceWithOverride(t, `{invalid`)
		data, err := svc.parsePricingData([]byte(gpt55OverrideCatalogJSON))
		require.NoError(t, err)
		require.Equal(t, 272000, data["gpt-5.5"].LongContextInputTokenThreshold)
	})
}

// 对真实出厂目录快照关闭 gpt-5.5 阶梯：计费视角阈值归零、基础价不变，
// 其他模型（gpt-5.4）的目录阶梯不受影响。
func TestPricingOverride_DisablesGPT55LadderOnDefaultCatalog(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)

	svc := newPricingServiceWithOverride(t, `{
		"gpt-5.5": {"long_context_input_token_threshold": 0},
		"gpt-5.5-2026-04-23": {"long_context_input_token_threshold": 0}
	}`)
	data, err := svc.parsePricingData(body)
	require.NoError(t, err)
	svc.pricingData = data
	billing := NewBillingService(&config.Config{}, svc)

	for _, model := range []string{"gpt-5.5", "gpt-5.5-2026-04-23"} {
		pricing, err := billing.GetModelPricing(model)
		require.NoError(t, err)
		require.Zero(t, pricing.LongContextInputThreshold, model)
		require.InDelta(t, 5e-6, pricing.InputPricePerToken, 1e-12, model)
		require.Empty(t, data[model].ContextPriceTiers, model)
		require.Zero(t, data[model].InputCostPerTokenAbove272K, model)
		require.Zero(t, data[model].OutputCostPerTokenAbove272K, model)
	}

	pricing, err := billing.GetModelPricing("gpt-5.4")
	require.NoError(t, err)
	require.Equal(t, 272000, pricing.LongContextInputThreshold, "其他模型的目录阶梯不受影响")
}

func TestWarnDroppedLongContextLadders_WarnsAtWarnLevelForIneffectiveReload(t *testing.T) {
	effective := &LiteLLMModelPricing{
		LongContextInputTokenThreshold:  272000,
		LongContextInputCostMultiplier:  2,
		LongContextOutputCostMultiplier: 1.5,
	}
	tests := []struct {
		name     string
		next     map[string]*LiteLLMModelPricing
		wantWarn bool
	}{
		{name: "model removed", next: map[string]*LiteLLMModelPricing{}, wantWarn: true},
		{name: "threshold removed", next: map[string]*LiteLLMModelPricing{"model": {
			LongContextInputCostMultiplier: 2, LongContextOutputCostMultiplier: 1.5,
		}}, wantWarn: true},
		{name: "multipliers no longer surcharge", next: map[string]*LiteLLMModelPricing{"model": {
			LongContextInputTokenThreshold: 272000, LongContextInputCostMultiplier: 1, LongContextOutputCostMultiplier: 1,
		}}, wantWarn: true},
		{name: "effective ladder retained", next: map[string]*LiteLLMModelPricing{"model": {
			LongContextInputTokenThreshold: 272000, LongContextInputCostMultiplier: 2, LongContextOutputCostMultiplier: 1,
		}}, wantWarn: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logSink, restore := captureStructuredLog(t)
			defer restore()

			warnDroppedLongContextLadders(map[string]*LiteLLMModelPricing{"model": effective}, tt.next)
			require.Equal(t, tt.wantWarn, logSink.ContainsMessageAtLevel("long-context ladder dropped", "warn"))
		})
	}
}

func TestParsePricingData_RejectsNegativePricesButKeepsZero(t *testing.T) {
	negativeFields := []string{
		"input_cost_per_token",
		"output_cost_per_token",
		"input_cost_per_token_priority",
		"cache_creation_input_token_cost",
		"cache_read_input_token_cost",
		"input_cost_per_token_above_128k_tokens",
		"output_cost_per_image",
	}
	for _, field := range negativeFields {
		t.Run(field, func(t *testing.T) {
			bad := map[string]any{
				"litellm_provider":      "test",
				"mode":                  "chat",
				"input_cost_per_token":  1e-6,
				"output_cost_per_token": 2e-6,
				field:                   -1.0,
			}
			catalog, err := json.Marshal(map[string]any{
				"control": map[string]any{
					"litellm_provider": "test", "mode": "chat",
					"input_cost_per_token": 1e-6, "output_cost_per_token": 2e-6,
				},
				"bad": bad,
			})
			require.NoError(t, err)

			data, err := (&PricingService{}).parsePricingData(catalog)
			require.NoError(t, err)
			require.Contains(t, data, "control")
			require.NotContains(t, data, "bad")
		})
	}

	data, err := (&PricingService{}).parsePricingData([]byte(`{
		"zero": {"litellm_provider": "test", "mode": "chat",
			"input_cost_per_token": 0, "output_cost_per_token": 2e-06,
			"cache_read_input_token_cost": 0,
			"input_cost_per_token_above_128k_tokens": 0}
	}`))
	require.NoError(t, err)
	require.Contains(t, data, "zero")
	require.Zero(t, data["zero"].InputCostPerToken)
	require.Zero(t, data["zero"].CacheReadInputTokenCost)
}
