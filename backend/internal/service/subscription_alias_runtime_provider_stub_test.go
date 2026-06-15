package service

import "context"

type subscriptionAliasRuntimeProviderStub struct {
	runtime SubscriptionEntitlementsRuntime
}

func (s subscriptionAliasRuntimeProviderStub) GetSubscriptionEntitlementsRuntime(context.Context) SubscriptionEntitlementsRuntime {
	return s.runtime
}
