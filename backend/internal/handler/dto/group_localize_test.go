package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func localizableGroup() *Group {
	return &Group{
		ID: 1, Name: "标准", Description: "号池",
		I18n: domain.GroupI18n{"en": {Name: "Standard", Description: "Pro + Max pool"}},
	}
}

func TestGroupLocalized_ReplacesTextAndDropsTranslations(t *testing.T) {
	g := localizableGroup().Localized("en")

	require.Equal(t, "Standard", g.Name)
	require.Equal(t, "Pro + Max pool", g.Description)

	raw, err := json.Marshal(g)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.NotContains(t, decoded, "i18n", "user-facing output must not carry the translation table")
}

func TestGroupLocalized_KeepsOriginalWhenLanguageHasNoTranslation(t *testing.T) {
	g := localizableGroup().Localized("zh")

	require.Equal(t, "标准", g.Name)
	require.Equal(t, "号池", g.Description)
	require.Nil(t, g.I18n)
}

func TestLocalized_NilSafe(t *testing.T) {
	require.Nil(t, (*Group)(nil).Localized("en"))
	require.Nil(t, (*APIKey)(nil).Localized("en"))
	require.Nil(t, (*RedeemCode)(nil).Localized("en"))
	require.Nil(t, (*UsageLog)(nil).Localized("en"))
	require.Nil(t, (*UserSubscription)(nil).Localized("en"))
	require.Nil(t, (*UserEntitlement)(nil).Localized("en"))
	require.Nil(t, (*UserSubscriptionAlias)(nil).Localized("en"))

	// 未携带分组的记录原样返回
	require.Nil(t, (&APIKey{}).Localized("en").Group)
}

func TestLocalized_NestedGroups(t *testing.T) {
	key := (&APIKey{Group: localizableGroup()}).Localized("en")
	require.Equal(t, "Standard", key.Group.Name)

	code := (&RedeemCode{Group: localizableGroup()}).Localized("en")
	require.Equal(t, "Standard", code.Group.Name)

	sub := (&UserSubscription{Group: localizableGroup()}).Localized("en")
	require.Equal(t, "Standard", sub.Group.Name)

	// 使用记录除自身分组外，还带着 API Key 与订阅各自的分组
	log := (&UsageLog{
		Group:        localizableGroup(),
		APIKey:       &APIKey{Group: localizableGroup()},
		Subscription: &UserSubscription{Group: localizableGroup()},
	}).Localized("en")
	require.Equal(t, "Standard", log.Group.Name)
	require.Equal(t, "Standard", log.APIKey.Group.Name)
	require.Equal(t, "Standard", log.Subscription.Group.Name)
	require.Nil(t, log.APIKey.Group.I18n)
	require.Nil(t, log.Subscription.Group.I18n)
}

func TestEntitlementGroupsLocalizedFromGrantsAndLegacyGroups(t *testing.T) {
	for _, withGrants := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy groups", true: "group grants"}[withGrants], func(t *testing.T) {
			legacyID := int64(7)
			primaryID := int64(2)
			groups := []service.Group{
				{ID: 1, Name: "标准", I18n: domain.GroupI18n{"en": {Name: "Standard"}}},
				{ID: 2, Name: "高级", I18n: domain.GroupI18n{"en": {Name: "Premium", Description: "Premium pool"}}},
			}
			ent := &service.SubscriptionEntitlement{LegacySubscriptionID: &legacyID, PrimaryGroupID: &primaryID, Groups: groups}
			if withGrants {
				ent.Groups = nil
				ent.GroupGrants = []service.SubscriptionEntitlementGroupGrant{
					{GroupID: 1, Enabled: true, Group: &groups[0]},
					{GroupID: 2, Enabled: true, Group: &groups[1]},
					{GroupID: 3, Enabled: false, Group: &service.Group{Name: "禁用"}},
				}
			}

			localized := UserEntitlementFromService(ent, time.Now()).Localized("en-US,en;q=0.9")
			require.Len(t, localized.Groups, 2)
			require.Equal(t, "Standard", localized.Groups[0].Name)
			require.Equal(t, "Premium", localized.Groups[1].Name)
			require.Nil(t, localized.Groups[0].I18n)

			alias := UserSubscriptionAliasFromEntitlement(ent).Localized("en")
			require.Equal(t, "Premium", alias.Group.Name)
			require.Equal(t, "Premium pool", alias.Group.Description)
			require.Equal(t, "Standard", alias.Groups[0].Name)
			require.Equal(t, "Premium", alias.Groups[1].Name)

			original := UserEntitlementFromService(ent, time.Now()).Localized("zh")
			require.Equal(t, "标准", original.Groups[0].Name)
			require.Equal(t, "高级", original.Groups[1].Name)
			require.Equal(t, "标准", groups[0].Name)
			require.Equal(t, "高级", groups[1].Name)
			require.NotEmpty(t, groups[0].I18n)

			for _, output := range []any{localized, alias} {
				raw, err := json.Marshal(output)
				require.NoError(t, err)
				require.NotContains(t, string(raw), `"i18n"`)
			}
		})
	}
}
