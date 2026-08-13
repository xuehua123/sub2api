//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// stubGroupRepoForAvailable 是 ListAvailable 测试用的 GroupRepository stub，
// 仅实现 ListActive；其他方法对本测试无关，返回零值即可。
// listActiveErr 非 nil 时，ListActive 返回该错误用于错误传播测试。
// listActiveCalls 记录调用次数，用于断言「失败短路时不再访问 groupRepo」等行为。
type stubGroupRepoForAvailable struct {
	activeGroups    []Group
	listActiveErr   error
	listActiveCalls int
}

func (s *stubGroupRepoForAvailable) ListActive(ctx context.Context) ([]Group, error) {
	s.listActiveCalls++
	if s.listActiveErr != nil {
		return nil, s.listActiveErr
	}
	return s.activeGroups, nil
}

func (s *stubGroupRepoForAvailable) Create(ctx context.Context, group *Group) error { return nil }
func (s *stubGroupRepoForAvailable) GetByID(ctx context.Context, id int64) (*Group, error) {
	return nil, nil
}
func (s *stubGroupRepoForAvailable) GetByIDLite(ctx context.Context, id int64) (*Group, error) {
	return nil, nil
}
func (s *stubGroupRepoForAvailable) Update(ctx context.Context, group *Group) error { return nil }
func (s *stubGroupRepoForAvailable) Delete(ctx context.Context, id int64) error     { return nil }
func (s *stubGroupRepoForAvailable) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	return nil, nil
}
func (s *stubGroupRepoForAvailable) List(ctx context.Context, params pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubGroupRepoForAvailable) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubGroupRepoForAvailable) ListActiveByPlatform(ctx context.Context, platform string) ([]Group, error) {
	return nil, nil
}
func (s *stubGroupRepoForAvailable) ExistsByName(ctx context.Context, name string) (bool, error) {
	return false, nil
}
func (s *stubGroupRepoForAvailable) GetAccountCount(ctx context.Context, groupID int64) (int64, int64, error) {
	return 0, 0, nil
}
func (s *stubGroupRepoForAvailable) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, nil
}
func (s *stubGroupRepoForAvailable) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	return nil, nil
}
func (s *stubGroupRepoForAvailable) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	return nil
}
func (s *stubGroupRepoForAvailable) UpdateSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error {
	return nil
}

// newAvailableChannelService 构造一个 ChannelService，channelRepo.ListAll 返回给定 channels，
// groupRepo 由参数决定。传入空 stub 表示「活跃分组列表为空」。
func newAvailableChannelService(channels []Channel, groupRepo GroupRepository) *ChannelService {
	repo := &mockChannelRepository{
		listAllFn: func(ctx context.Context) ([]Channel, error) { return channels, nil },
	}
	return NewChannelService(repo, groupRepo, nil, nil, nil)
}

func TestListAvailable_EmptyActiveGroups_NoGroupsAttached(t *testing.T) {
	// 活跃分组列表为空时，渠道的 Groups 应为空切片，不报错。
	channels := []Channel{{
		ID:       1,
		Name:     "chA",
		Status:   StatusActive,
		GroupIDs: []int64{10, 20},
	}}
	svc := newAvailableChannelService(channels, &stubGroupRepoForAvailable{})
	out, err := svc.ListAvailable(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Empty(t, out[0].Groups)
}

func TestListAvailable_InactiveGroupIDSilentlyDropped(t *testing.T) {
	// 渠道 GroupIDs 中引用的 group 未出现在 ListActive 结果中（已停用或删除），应被静默丢弃。
	channels := []Channel{{
		ID:       1,
		Name:     "chA",
		Status:   StatusActive,
		GroupIDs: []int64{1, 99},
	}}
	groupRepo := &stubGroupRepoForAvailable{
		activeGroups: []Group{{ID: 1, Name: "g1", Platform: "anthropic"}},
	}
	svc := newAvailableChannelService(channels, groupRepo)
	out, err := svc.ListAvailable(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Groups, 1)
	require.Equal(t, int64(1), out[0].Groups[0].ID)
}

func TestListAvailable_SortedByName(t *testing.T) {
	channels := []Channel{
		{ID: 1, Name: "beta"},
		{ID: 2, Name: "Alpha"},
		{ID: 3, Name: "charlie"},
	}
	svc := newAvailableChannelService(channels, &stubGroupRepoForAvailable{})
	out, err := svc.ListAvailable(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 3)
	require.Equal(t, "Alpha", out[0].Name)
	require.Equal(t, "beta", out[1].Name)
	require.Equal(t, "charlie", out[2].Name)
}

func TestListAvailable_ListAllErrorPropagates(t *testing.T) {
	// ListAll 返回错误时 ListAvailable 应直接返回包装后的错误，且不再访问 groupRepo（短路）。
	sentinel := errors.New("list-all-boom")
	repo := &mockChannelRepository{
		listAllFn: func(ctx context.Context) ([]Channel, error) { return nil, sentinel },
	}
	groupRepo := &stubGroupRepoForAvailable{}
	svc := NewChannelService(repo, groupRepo, nil, nil, nil)
	out, err := svc.ListAvailable(context.Background())
	require.Nil(t, out)
	require.ErrorIs(t, err, sentinel)
	require.Contains(t, err.Error(), "list channels", "wrap 前缀缺失，可能 %w 被改为 %v")
	require.Equal(t, 0, groupRepo.listActiveCalls, "ListAll 失败后不应再调用 groupRepo.ListActive")
}

func TestListAvailable_ListActiveErrorPropagates(t *testing.T) {
	// groupRepo.ListActive 返回错误时 ListAvailable 应直接返回包装后的错误。
	sentinel := errors.New("list-active-boom")
	svc := newAvailableChannelService(
		[]Channel{{ID: 1, Name: "chA"}},
		&stubGroupRepoForAvailable{listActiveErr: sentinel},
	)
	out, err := svc.ListAvailable(context.Background())
	require.Nil(t, out)
	require.ErrorIs(t, err, sentinel)
	require.Contains(t, err.Error(), "list active groups", "wrap 前缀缺失，可能 %w 被改为 %v")
}

func TestListAvailable_DefaultsEmptyBillingModelSource(t *testing.T) {
	// 渠道 BillingModelSource 为空时应回填为 BillingModelSourceChannelMapped，
	// 显式值应原样保留（由 service 层统一处理，避免各 handler 重复默认逻辑）。
	channels := []Channel{
		{ID: 1, Name: "empty", BillingModelSource: ""},
		{ID: 2, Name: "explicit", BillingModelSource: BillingModelSourceUpstream},
	}
	svc := newAvailableChannelService(channels, &stubGroupRepoForAvailable{})
	out, err := svc.ListAvailable(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 2)

	// 按 Name 查找，避免依赖排序副作用。
	byName := make(map[string]string, len(out))
	for _, ch := range out {
		byName[ch.Name] = ch.BillingModelSource
	}
	require.Equal(t, BillingModelSourceChannelMapped, byName["empty"])
	require.Equal(t, BillingModelSourceUpstream, byName["explicit"])
}

func TestPricingNeedsFallback(t *testing.T) {
	tests := []struct {
		name string
		in   *ChannelModelPricing
		want bool
	}{
		{"nil", nil, true},
		{"empty struct", &ChannelModelPricing{BillingMode: BillingModeToken}, true},
		{"all-empty intervals", &ChannelModelPricing{
			BillingMode: BillingModeImage,
			Intervals:   []PricingInterval{{TierLabel: "1K"}, {TierLabel: "2K"}},
		}, true},
		{"flat input set", &ChannelModelPricing{InputPrice: testPtrFloat64(3e-6)}, false},
		{"image input zero set", &ChannelModelPricing{ImageInputPrice: testPtrFloat64(0)}, false},
		{"flat per_request set", &ChannelModelPricing{PerRequestPrice: testPtrFloat64(0.04)}, false},
		{"interval with price", &ChannelModelPricing{
			Intervals: []PricingInterval{{TierLabel: "1K", PerRequestPrice: testPtrFloat64(0.04)}},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, pricingNeedsFallback(tt.in))
		})
	}
}

func TestSynthesizePricingFromLiteLLM_TokenMode(t *testing.T) {
	lp := &LiteLLMModelPricing{
		Mode:                        "chat",
		InputCostPerToken:           3e-6,
		OutputCostPerToken:          1.5e-5,
		CacheCreationInputTokenCost: 3.75e-6,
		CacheReadInputTokenCost:     3e-7,
		InputCostPerImageToken:      7e-6,
	}
	got := synthesizePricingFromLiteLLM(lp, nil)
	require.NotNil(t, got)
	require.Equal(t, BillingModeToken, got.BillingMode)
	require.NotNil(t, got.InputPrice)
	require.InDelta(t, 3e-6, *got.InputPrice, 1e-12)
	require.NotNil(t, got.CacheReadPrice)
	require.NotNil(t, got.ImageInputPrice)
	require.InDelta(t, 7e-6, *got.ImageInputPrice, 1e-12)
}

func TestSynthesizePricingFromLiteLLM_ImageGenerationMode(t *testing.T) {
	// LiteLLM mode=image_generation 且渠道未声明模式时，按 image 合成。
	lp := &LiteLLMModelPricing{
		Mode:                    "image_generation",
		OutputCostPerImageToken: 4e-5,
	}
	got := synthesizePricingFromLiteLLM(lp, nil)
	require.NotNil(t, got)
	require.Equal(t, BillingModeImage, got.BillingMode)
	require.Nil(t, got.PerRequestPrice)
	require.NotNil(t, got.ImageOutputPrice)
}

func TestSynthesizePricingFromLiteLLM_RespectsExistingChannelMode(t *testing.T) {
	// admin UI 选了 per_request 但没填价：LiteLLM 数据按 per_request 合成,
	// 即便 LiteLLM 标的是 chat 模式也尊重渠道选择。
	lp := &LiteLLMModelPricing{
		Mode:               "chat",
		InputCostPerToken:  5e-6,
		OutputCostPerImage: 0.04,
	}
	existing := &ChannelModelPricing{BillingMode: BillingModePerRequest}
	got := synthesizePricingFromLiteLLM(lp, existing)
	require.NotNil(t, got)
	require.Equal(t, BillingModePerRequest, got.BillingMode)
	require.NotNil(t, got.PerRequestPrice)
	require.InDelta(t, 0.04, *got.PerRequestPrice, 1e-12)
}

func TestFillGlobalPricingFallback_NilPricing(t *testing.T) {
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"claude-opus-4-5": {Mode: "chat", InputCostPerToken: 5e-6},
	})
	svc := &ChannelService{pricingService: pricingSvc}

	models := []SupportedModel{
		{Name: "claude-opus-4-5", Platform: "anthropic"},
	}
	svc.fillGlobalPricingFallback(models)
	require.NotNil(t, models[0].Pricing)
	require.NotNil(t, models[0].Pricing.InputPrice)
	require.InDelta(t, 5e-6, *models[0].Pricing.InputPrice, 1e-12)
}

func TestFillGlobalPricingFallback_EmptyPricingFillsFromLiteLLM(t *testing.T) {
	// 核心场景：admin UI 建了 pricing 条目（image 模式）但没填价，应走 LiteLLM 兜底。
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"gpt-image-1": {
			Mode:                    "image_generation",
			OutputCostPerImageToken: 4e-5,
		},
	})
	svc := &ChannelService{pricingService: pricingSvc}

	models := []SupportedModel{
		{
			Name:     "gpt-image-1",
			Platform: "openai",
			Pricing: &ChannelModelPricing{
				BillingMode: BillingModeImage,
				Intervals:   []PricingInterval{{TierLabel: "1K"}, {TierLabel: "2K"}},
			},
		},
	}
	svc.fillGlobalPricingFallback(models)
	require.NotNil(t, models[0].Pricing)
	require.Equal(t, BillingModeImage, models[0].Pricing.BillingMode)
	require.NotNil(t, models[0].Pricing.ImageOutputPrice)
	require.InDelta(t, 4e-5, *models[0].Pricing.ImageOutputPrice, 1e-12)
}

func TestFillGlobalPricingFallback_EnrichesPartialTokenPricingWithoutOverwritingExplicitValues(t *testing.T) {
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"served-model": {
			Mode: "chat", InputCostPerToken: 1e-6, OutputCostPerToken: 5e-6,
			CacheReadInputTokenCost: 0.1e-6, InputCostPerImageToken: 7e-6,
		},
	})
	svc := &ChannelService{pricingService: pricingSvc}

	zero := 0.0
	existing := &ChannelModelPricing{
		BillingMode:     BillingModeToken,
		InputPrice:      testPtrFloat64(9e-9),
		ImageInputPrice: &zero,
	}
	models := []SupportedModel{
		{Name: "served-model", Platform: "anthropic", Pricing: existing},
	}
	svc.fillGlobalPricingFallback(models)
	require.NotSame(t, existing, models[0].Pricing)
	require.InDelta(t, 9e-9, *models[0].Pricing.InputPrice, 1e-12)
	require.InDelta(t, 5e-6, *models[0].Pricing.OutputPrice, 1e-12)
	require.InDelta(t, 0.1e-6, *models[0].Pricing.CacheReadPrice, 1e-12)
	require.NotNil(t, models[0].Pricing.ImageInputPrice)
	require.Zero(t, *models[0].Pricing.ImageInputPrice, "explicit zero image input must survive merge")
	require.NotNil(t, models[0].Pricing.ImageOutputPrice)
	require.Zero(t, *models[0].Pricing.ImageOutputPrice, "missing channel image output is runtime-explicit zero")
}

func newStubPricingServiceFromMap(data map[string]*LiteLLMModelPricing) *PricingService {
	return &PricingService{pricingData: data}
}

func TestListAvailable_PricingByGroupUsesExactWildcardAndExplicitZero(t *testing.T) {
	channelPrice := 9e-6
	wildcardPrice := 2e-6
	exactZero := 0.0
	groups := []Group{
		{
			ID: 1, Name: "exact", Platform: PlatformOpenAI,
			ModelPricing: []ChannelModelPricing{
				{Models: []string{"gpt-*"}, BillingMode: BillingModeToken, InputPrice: &wildcardPrice},
				{Models: []string{"GPT-5.6-SOL"}, BillingMode: BillingModeToken, InputPrice: &exactZero},
			},
		},
		{
			ID: 2, Name: "wildcard", Platform: PlatformOpenAI,
			ModelPricing: []ChannelModelPricing{
				{Models: []string{"gpt-*"}, BillingMode: BillingModeToken, InputPrice: &wildcardPrice},
			},
		},
		{ID: 3, Name: "channel-fallback", Platform: PlatformOpenAI},
	}
	channels := []Channel{{
		ID: 1, Name: "openai", Status: StatusActive, GroupIDs: []int64{1, 2, 3},
		ModelPricing: []ChannelModelPricing{{
			Platform: PlatformOpenAI, Models: []string{"gpt-5.6-sol"},
			BillingMode: BillingModeToken, InputPrice: &channelPrice,
		}},
	}}

	out, err := newAvailableChannelService(
		channels,
		&stubGroupRepoForAvailable{activeGroups: groups},
	).ListAvailable(context.Background())

	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].SupportedModels, 1)
	byGroup := out[0].SupportedModels[0].PricingByGroup
	require.NotNil(t, byGroup[1].InputPrice)
	require.Zero(t, *byGroup[1].InputPrice, "exact rule and explicit zero must win")
	require.InDelta(t, wildcardPrice, *byGroup[2].InputPrice, 1e-12)
	require.InDelta(t, channelPrice, *byGroup[3].InputPrice, 1e-12)
	require.NotSame(t, out[0].SupportedModels[0].Pricing, byGroup[3])
}

func TestListAvailable_GroupPricingFollowsRequestedVsChannelMappedSource(t *testing.T) {
	requestedPrice := 1e-6
	mappedPrice := 8e-6
	group := Group{
		ID: 1, Name: "g", Platform: PlatformOpenAI,
		ModelPricing: []ChannelModelPricing{
			{Models: []string{"public-alias"}, InputPrice: &requestedPrice},
			{Models: []string{"provider-model"}, InputPrice: &mappedPrice},
		},
	}
	base := Channel{
		Status: StatusActive, GroupIDs: []int64{1},
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"public-alias": "provider-model"},
		},
		ModelPricing: []ChannelModelPricing{{
			Platform: PlatformOpenAI, Models: []string{"provider-model"}, InputPrice: &mappedPrice,
		}},
	}
	requested := base
	requested.ID, requested.Name = 1, "requested"
	requested.BillingModelSource = BillingModelSourceRequested
	mapped := base
	mapped.ID, mapped.Name = 2, "mapped"
	mapped.BillingModelSource = BillingModelSourceChannelMapped

	out, err := newAvailableChannelService(
		[]Channel{requested, mapped},
		&stubGroupRepoForAvailable{activeGroups: []Group{group}},
	).ListAvailable(context.Background())

	require.NoError(t, err)
	require.Len(t, out, 2)
	aliasPrice := func(ch AvailableChannel) float64 {
		for _, model := range ch.SupportedModels {
			if model.Name == "public-alias" {
				require.NotNil(t, model.PricingByGroup[1])
				require.NotNil(t, model.PricingByGroup[1].InputPrice)
				return *model.PricingByGroup[1].InputPrice
			}
		}
		t.Fatalf("public alias missing from channel %q", ch.Name)
		return 0
	}
	require.InDelta(t, mappedPrice, aliasPrice(out[0]), 1e-12)
	require.InDelta(t, requestedPrice, aliasPrice(out[1]), 1e-12)
}

func TestGroupModelPricingForDisplay_LongContextSwitchMatchesRuntimePolicy(t *testing.T) {
	baseInput := 2e-6
	baseOutput := 10e-6
	officialImageInput := 7e-6
	officialImageOutput := 20e-6
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"gpt-5.6-sol": {
			InputCostPerToken:               3e-6,
			OutputCostPerToken:              15e-6,
			InputCostPerImageToken:          officialImageInput,
			OutputCostPerImageToken:         officialImageOutput,
			LongContextInputTokenThreshold:  272000,
			LongContextInputCostMultiplier:  2,
			LongContextOutputCostMultiplier: 1.5,
		},
	})
	svc := &ChannelService{pricingService: pricingSvc}
	group := Group{
		LongContextPricingEnabled: true,
		ModelPricing: []ChannelModelPricing{{
			Models: []string{"gpt-5.6-sol"}, BillingMode: BillingModeToken,
			InputPrice: &baseInput, OutputPrice: &baseOutput,
		}},
	}

	enabled, matched := svc.GroupModelPricingForDisplay(&group, "gpt-5.6-sol")

	require.True(t, matched)
	require.Len(t, enabled.Intervals, 4)
	require.Equal(t, 272000, *enabled.Intervals[0].MaxTokens)
	require.InDelta(t, baseInput, *enabled.Intervals[0].InputPrice, 1e-12)
	require.InDelta(t, baseInput*2, *enabled.Intervals[1].InputPrice, 1e-12)
	require.InDelta(t, baseOutput*1.5, *enabled.Intervals[1].OutputPrice, 1e-12)
	require.NotNil(t, enabled.ImageInputPrice)
	require.InDelta(t, baseInput, *enabled.ImageInputPrice, 1e-12, "missing group image input falls back to effective text input")
	require.NotNil(t, enabled.ImageOutputPrice)
	require.Zero(t, *enabled.ImageOutputPrice, "missing group image output is an explicit runtime zero")

	group.LongContextPricingEnabled = false
	disabled, matched := svc.GroupModelPricingForDisplay(&group, "gpt-5.6-sol")
	require.True(t, matched)
	require.Len(t, disabled.Intervals, 2)
	require.Equal(t, "基础", disabled.Intervals[0].TierLabel)
	require.Equal(t, "Fast", disabled.Intervals[1].TierLabel)
	require.InDelta(t, baseInput, *disabled.InputPrice, 1e-12)
}

func TestResolveGroupModelPricingForDisplay_PreservesRuntimeFastTiers(t *testing.T) {
	baseInput := 2e-6
	baseOutput := 10e-6
	baseCacheWrite := 1e-6
	baseCacheRead := 0.2e-6
	zero := 0.0
	groupFor := func(enabled bool, cacheRead *float64) *Group {
		return &Group{
			LongContextPricingEnabled: enabled,
			ModelPricing: []ChannelModelPricing{{
				Models:          []string{"*"},
				BillingMode:     BillingModeToken,
				InputPrice:      &baseInput,
				OutputPrice:     &baseOutput,
				CacheWritePrice: &baseCacheWrite,
				CacheReadPrice:  cacheRead,
			}},
		}
	}
	byLabel := func(t *testing.T, pricing *ChannelModelPricing) map[string]PricingInterval {
		t.Helper()
		out := make(map[string]PricingInterval, len(pricing.Intervals))
		for _, interval := range pricing.Intervals {
			out[interval.TierLabel] = interval
		}
		return out
	}

	t.Run("gpt-5.5 enabled keeps base long fast and fast-long", func(t *testing.T) {
		got, ok := ResolveGroupModelPricingForDisplay(groupFor(true, &baseCacheRead), "gpt-5.5", nil)
		require.True(t, ok)
		require.Len(t, got.Intervals, 4)
		tiers := byLabel(t, got)
		require.InDelta(t, baseInput*2, *tiers["长上下文"].InputPrice, 1e-12)
		require.InDelta(t, baseCacheWrite*2, *tiers["长上下文"].CacheWritePrice, 1e-12)
		require.InDelta(t, baseInput*openAIGPT55PriorityMultiplier, *tiers["Fast"].InputPrice, 1e-12)
		require.InDelta(t, baseCacheRead*openAIGPT55PriorityMultiplier, *tiers["Fast"].CacheReadPrice, 1e-12)
		require.InDelta(t, baseInput*2*openAIGPT55PriorityMultiplier, *tiers["Fast 长上下文"].InputPrice, 1e-12)
	})

	t.Run("long-context switch does not hide gpt-5.5 fast", func(t *testing.T) {
		got, ok := ResolveGroupModelPricingForDisplay(groupFor(false, &baseCacheRead), "gpt-5.5", nil)
		require.True(t, ok)
		require.Len(t, got.Intervals, 2)
		tiers := byLabel(t, got)
		require.Contains(t, tiers, "基础")
		require.Contains(t, tiers, "Fast")
		require.NotContains(t, tiers, "长上下文")
		require.NotContains(t, tiers, "Fast 长上下文")
	})

	t.Run("gpt-5.4 fast floors and priority long uses standard long", func(t *testing.T) {
		got, ok := ResolveGroupModelPricingForDisplay(groupFor(true, &baseCacheRead), "gpt-5.4", nil)
		require.True(t, ok)
		tiers := byLabel(t, got)
		require.InDelta(t, openAIGPT54PriorityInputPrice, *tiers["Fast"].InputPrice, 1e-12)
		require.InDelta(t, openAIGPT54PriorityOutputPrice, *tiers["Fast"].OutputPrice, 1e-12)
		require.InDelta(t, openAIGPT54PriorityCacheReadPrice, *tiers["Fast"].CacheReadPrice, 1e-12)
		require.InDelta(t, *tiers["长上下文"].InputPrice, *tiers["Fast 长上下文"].InputPrice, 1e-12)
		require.InDelta(t, *tiers["长上下文"].CacheWritePrice, *tiers["Fast 长上下文"].CacheWritePrice, 1e-12)
	})

	t.Run("catalog priority overlays group values and preserves explicit zero cache", func(t *testing.T) {
		official := &LiteLLMModelPricing{
			InputCostPerToken:                   3e-6,
			InputCostPerTokenPriority:           6e-6,
			OutputCostPerToken:                  15e-6,
			OutputCostPerTokenPriority:          30e-6,
			CacheCreationInputTokenCost:         2e-6,
			CacheCreationInputTokenCostPriority: 4e-6,
			CacheReadInputTokenCost:             0.3e-6,
			CacheReadInputTokenCostPriority:     0.6e-6,
			LongContextInputTokenThreshold:      200000,
			LongContextInputCostMultiplier:      2,
			LongContextOutputCostMultiplier:     1.5,
			LongContextCacheReadCostMultiplier:  2,
			SupportsServiceTier:                 true,
		}
		got, ok := ResolveGroupModelPricingForDisplay(groupFor(true, &zero), "custom-priority", official)
		require.True(t, ok)
		tiers := byLabel(t, got)
		// Explicit group input/cache-write values also replace their runtime
		// priority fields; untouched output keeps the catalog priority rate.
		require.InDelta(t, baseInput, *tiers["Fast"].InputPrice, 1e-12)
		require.InDelta(t, baseCacheWrite, *tiers["Fast"].CacheWritePrice, 1e-12)
		require.InDelta(t, baseOutput, *tiers["Fast"].OutputPrice, 1e-12)
		require.NotNil(t, tiers["Fast"].CacheReadPrice)
		require.Zero(t, *tiers["Fast"].CacheReadPrice)
		require.NotNil(t, tiers["Fast 长上下文"].CacheReadPrice)
		require.Zero(t, *tiers["Fast 长上下文"].CacheReadPrice)
	})

	t.Run("ordinary group override does not invent fast capability", func(t *testing.T) {
		got, ok := ResolveGroupModelPricingForDisplay(groupFor(false, &baseCacheRead), "claude-ordinary", &LiteLLMModelPricing{
			InputCostPerToken:  3e-6,
			OutputCostPerToken: 15e-6,
		})
		require.True(t, ok)
		require.Empty(t, got.Intervals)
	})

	t.Run("grok inclusive threshold starts long tier at exactly 200k", func(t *testing.T) {
		got, ok := ResolveGroupModelPricingForDisplay(groupFor(true, &baseCacheRead), "grok-4.5", &LiteLLMModelPricing{
			InputCostPerToken:                  2e-6,
			OutputCostPerToken:                 6e-6,
			LongContextInputTokenThreshold:     200000,
			LongContextInputCostMultiplier:     2,
			LongContextOutputCostMultiplier:    2,
			LongContextCacheReadCostMultiplier: 2,
		})
		require.True(t, ok)
		require.Len(t, got.Intervals, 2)
		require.Equal(t, 199999, *got.Intervals[0].MaxTokens)
		require.Equal(t, 199999, got.Intervals[1].MinTokens)
	})
}

func TestPricingForGroupDisplay_NoGroupOverrideStillProjectsRuntimeTiers(t *testing.T) {
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"gpt-5.6-sol": {
			InputCostPerToken:               2e-6,
			OutputCostPerToken:              8e-6,
			LongContextInputTokenThreshold:  272000,
			LongContextInputCostMultiplier:  2,
			LongContextOutputCostMultiplier: 1.5,
		},
	})
	svc := &ChannelService{
		pricingService: pricingSvc,
		billingService: NewBillingService(&config.Config{}, pricingSvc),
	}

	got, matched := svc.PricingForGroupDisplay(&Group{
		Platform:                  PlatformOpenAI,
		LongContextPricingEnabled: true,
	}, "gpt-5.6-sol", nil)

	require.False(t, matched)
	require.NotNil(t, got)
	require.Len(t, got.Intervals, 4)
	require.Equal(t, "基础", got.Intervals[0].TierLabel)
	require.Zero(t, got.Intervals[0].MinTokens)
	require.True(t, got.Intervals[1].RequiresAccountLongContext)
	require.Equal(t, "Fast", got.Intervals[2].TierLabel)
	require.True(t, got.Intervals[3].RequiresAccountLongContext)
}

func TestPricingForGroupDisplay_ChannelIntervalsFollowGroupSwitch(t *testing.T) {
	inputBase, inputLong := 2e-6, 4e-6
	raw := &ChannelModelPricing{
		Platform: PlatformOpenAI, BillingMode: BillingModeToken,
		Intervals: []PricingInterval{
			{MinTokens: 0, TierLabel: "base", InputPrice: &inputBase},
			{MinTokens: 32000, TierLabel: "long", InputPrice: &inputLong},
		},
	}
	svc := &ChannelService{billingService: NewBillingService(&config.Config{}, nil)}

	disabled, _ := svc.PricingForGroupDisplay(&Group{Platform: PlatformOpenAI}, "unknown-local", raw)
	require.NotNil(t, disabled)
	require.InDelta(t, inputBase, *disabled.InputPrice, 1e-12)
	require.Empty(t, disabled.Intervals)

	enabled, _ := svc.PricingForGroupDisplay(&Group{Platform: PlatformOpenAI, LongContextPricingEnabled: true}, "gpt-5.5", raw)
	require.Len(t, enabled.Intervals, 4)
	require.Equal(t, "base", enabled.Intervals[0].TierLabel)
	require.Equal(t, "Fast base", enabled.Intervals[1].TierLabel)
	require.InDelta(t, inputBase*openAIGPT55PriorityMultiplier, *enabled.Intervals[1].InputPrice, 1e-12)
}

func TestPricingForGroupDisplay_UnknownGroupExplicitPriceAndZeroImageInput(t *testing.T) {
	input, zero := 3e-6, 0.0
	svc := &ChannelService{billingService: NewBillingService(&config.Config{}, nil)}
	got, matched := svc.PricingForGroupDisplay(&Group{
		Platform: PlatformOpenAI,
		ModelPricing: []ChannelModelPricing{{
			Models: []string{"unknown-local"}, InputPrice: &input, ImageInputPrice: &zero,
		}},
	}, "unknown-local", nil)
	require.True(t, matched)
	require.NotNil(t, got)
	require.InDelta(t, input, *got.InputPrice, 1e-12)
	require.InDelta(t, input, *got.ImageInputPrice, 1e-12)
}

func TestPricingForGroupDisplay_GPT56IntervalUsesGenericCachePolicy(t *testing.T) {
	input := 4e-6
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"gpt-5.6-sol": {
			InputCostPerToken:                   2e-6,
			OutputCostPerToken:                  8e-6,
			CacheCreationInputTokenCost:         2.5e-6,
			CacheCreationInputTokenCostAbove1hr: 4e-6,
		},
	})
	svc := &ChannelService{
		pricingService: pricingSvc,
		billingService: NewBillingService(&config.Config{}, pricingSvc),
	}
	raw := &ChannelModelPricing{BillingMode: BillingModeToken, Intervals: []PricingInterval{{
		TierLabel: "base", InputPrice: &input,
	}}}

	got, _ := svc.PricingForGroupDisplay(&Group{Platform: PlatformOpenAI, LongContextPricingEnabled: true}, "gpt-5.6-sol", raw)

	require.Len(t, got.Intervals, 2)
	base := got.Intervals[0]
	require.NotNil(t, base.CacheWritePrice)
	require.InDelta(t, input*1.25, *base.CacheWritePrice, 1e-12)
	require.Nil(t, base.CacheWrite5mPrice)
	require.Nil(t, base.CacheWrite1hPrice)
}

func TestFillGlobalPricingFallbackTracksOfficialAndFallbackSources(t *testing.T) {
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"catalog-model": {InputCostPerToken: 2e-6},
	})
	svc := &ChannelService{
		pricingService: pricingSvc,
		billingService: NewBillingService(&config.Config{}, pricingSvc),
	}
	models := []SupportedModel{
		{Name: "catalog-model", BillingModel: "catalog-model"},
		{Name: "gpt-5.5", BillingModel: "gpt-5.5"},
	}

	svc.fillGlobalPricingFallback(models)

	require.Equal(t, PricingSourceOfficial, models[0].PricingSource)
	require.Equal(t, PricingSourceFallback, models[1].PricingSource)
}

func TestListAvailable_MappedAliasUsesBillingModelForGlobalFallback(t *testing.T) {
	globalPrice := 4e-6
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"provider-model": {InputCostPerToken: globalPrice},
	})
	channel := Channel{
		ID: 1, Name: "mapped", Status: StatusActive, GroupIDs: []int64{10},
		BillingModelSource: BillingModelSourceChannelMapped,
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"public-alias": "provider-model"},
		},
	}
	svc := newAvailableChannelService(
		[]Channel{channel},
		&stubGroupRepoForAvailable{activeGroups: []Group{{ID: 10, Name: "g", Platform: PlatformOpenAI}}},
	)
	svc.pricingService = pricingSvc

	out, err := svc.ListAvailable(context.Background())

	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].SupportedModels, 1)
	model := out[0].SupportedModels[0]
	require.Equal(t, "public-alias", model.Name)
	require.Equal(t, "provider-model", model.BillingModel)
	require.NotNil(t, model.Pricing)
	require.InDelta(t, globalPrice, *model.Pricing.InputPrice, 1e-12)
	require.InDelta(t, globalPrice, *model.PricingByGroup[10].InputPrice, 1e-12)
}
