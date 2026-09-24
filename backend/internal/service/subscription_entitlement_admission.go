package service

import (
	"context"
	"sync"
)

type entitlementGenerationAdmissionKey struct{}
type entitlementGenerationAdmission struct {
	mu        sync.Mutex
	completed bool
	prepared  bool
	id        int64
	admit     func(context.Context, float64) (*APIKeyEntitlementAuthResult, error)
}

// Carry admission into validated HTTP handlers and asynchronous image execution.
// The callback captures services and the resolved key, never a pooled gin.Context.
func (s *APIKeyService) WithEntitlementGenerationAdmission(ctx context.Context, key *APIKey, id int64) context.Context {
	return context.WithValue(ctx, entitlementGenerationAdmissionKey{}, &entitlementGenerationAdmission{
		id: id, admit: func(callCtx context.Context, cost float64) (*APIKeyEntitlementAuthResult, error) {
			return s.AdmitEntitlementGeneration(callCtx, key, id, cost)
		},
	})
}

func HasEntitlementGenerationAdmission(ctx context.Context) bool {
	_, ok := ctx.Value(entitlementGenerationAdmissionKey{}).(*entitlementGenerationAdmission)
	return ok
}

func PrepareEntitlementGenerationAdmission(ctx context.Context, ent *SubscriptionEntitlement) error {
	state, ok := ctx.Value(entitlementGenerationAdmissionKey{}).(*entitlementGenerationAdmission)
	if !ok {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if ent != nil && ent.ID != state.id {
		return ErrSubscriptionEntitlementNotFound
	}
	state.prepared = true
	return nil
}

// CompleteEntitlementGenerationAdmission is called only after generation validation
// and rate limits. Retries in the same request never consume another future cycle.
func CompleteEntitlementGenerationAdmission(ctx context.Context, ent *SubscriptionEntitlement, cost float64) error {
	state, ok := ctx.Value(entitlementGenerationAdmissionKey{}).(*entitlementGenerationAdmission)
	if !ok {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.prepared {
		return ErrBillingServiceUnavailable
	}
	if state.completed {
		return nil
	}
	if ent != nil && ent.ID != state.id {
		return ErrSubscriptionEntitlementNotFound
	}
	result, err := state.admit(ctx, cost)
	if err != nil {
		return err
	}
	if result == nil || result.Entitlement == nil {
		return ErrSubscriptionEntitlementNotFound
	}
	if ent != nil {
		*ent = *result.Entitlement
	}
	state.completed = true
	return nil
}
