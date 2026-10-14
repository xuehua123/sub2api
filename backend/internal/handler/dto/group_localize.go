package dto

// 用户侧接口在输出前调用 Localized：把分组的名称与描述换成请求首选语言
// （Accept-Language）的版本，并去掉译文表。管理端接口不调用，保留原字段与译文表，
// 分组编辑表单依赖原字段回写。

// Localized 就地本地化分组并返回自身。
func (g *Group) Localized(acceptLanguage string) *Group {
	if g == nil {
		return nil
	}
	g.Name, g.Description = g.I18n.Localize(acceptLanguage, g.Name, g.Description)
	g.I18n = nil
	return g
}

// Localized 本地化 API Key 所属分组并返回自身。
func (k *APIKey) Localized(acceptLanguage string) *APIKey {
	if k != nil {
		k.Group.Localized(acceptLanguage)
	}
	return k
}

// Localized 本地化兑换码关联分组并返回自身。
func (r *RedeemCode) Localized(acceptLanguage string) *RedeemCode {
	if r != nil {
		r.Group.Localized(acceptLanguage)
	}
	return r
}

// Localized 本地化使用记录关联的分组（含其 API Key 与订阅各自携带的分组）并返回自身。
func (l *UsageLog) Localized(acceptLanguage string) *UsageLog {
	if l != nil {
		l.Group.Localized(acceptLanguage)
		l.APIKey.Localized(acceptLanguage)
		l.Subscription.Localized(acceptLanguage)
	}
	return l
}

// Localized 本地化订阅关联分组并返回自身。
func (s *UserSubscription) Localized(acceptLanguage string) *UserSubscription {
	if s != nil {
		s.Group.Localized(acceptLanguage)
	}
	return s
}

// Localized 本地化权益关联的全部分组并返回自身。
func (e *UserEntitlement) Localized(acceptLanguage string) *UserEntitlement {
	if e != nil {
		localizeEntitlementGroups(e.Groups, acceptLanguage)
	}
	return e
}

// Localized 本地化兼容订阅的主分组与全部权益分组并返回自身。
func (s *UserSubscriptionAlias) Localized(acceptLanguage string) *UserSubscriptionAlias {
	if s != nil {
		s.Group.Localized(acceptLanguage)
		localizeEntitlementGroups(s.Groups, acceptLanguage)
	}
	return s
}

// LocalizeUserEntitlements 本地化已经映射好的用户权益列表。
func LocalizeUserEntitlements(entitlements []UserEntitlement, acceptLanguage string) []UserEntitlement {
	for i := range entitlements {
		entitlements[i].Localized(acceptLanguage)
	}
	return entitlements
}

// LocalizeUserSubscriptionAliases 本地化已经映射好的兼容订阅列表。
func LocalizeUserSubscriptionAliases(subscriptions []UserSubscriptionAlias, acceptLanguage string) []UserSubscriptionAlias {
	for i := range subscriptions {
		subscriptions[i].Localized(acceptLanguage)
	}
	return subscriptions
}

func localizeEntitlementGroups(groups []UserEntitlementGroup, acceptLanguage string) {
	for i := range groups {
		groups[i].Name = groups[i].I18n.LocalizeName(acceptLanguage, groups[i].Name)
		groups[i].I18n = nil
	}
}
