package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrUsageBillingRequestIDRequired = errors.New("usage billing request_id is required")
var ErrUsageBillingRequestConflict = errors.New("usage billing request fingerprint conflict")

// UsageBillingCommand describes one billable request that must be applied at most once.
type UsageBillingCommand struct {
	RequestID          string
	APIKeyID           int64
	RequestFingerprint string
	RequestPayloadHash string

	UserID              int64
	AccountID           int64
	SubscriptionID      *int64
	EntitlementID       *int64
	AccountType         string
	Model               string
	ServiceTier         string
	ReasoningEffort     string
	BillingType         int8
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	ImageCount          int
	MediaType           string

	BalanceCost                float64
	SubscriptionCost           float64
	EntitlementBalanceFallback bool
	AllowEntitlementOverage    bool
	APIKeyQuotaCost            float64
	APIKeyRateLimitCost        float64
	AccountQuotaCost           float64
}

func (c *UsageBillingCommand) Normalize() {
	if c == nil {
		return
	}
	c.RequestID = strings.TrimSpace(c.RequestID)
	if strings.TrimSpace(c.RequestFingerprint) == "" {
		c.RequestFingerprint = buildUsageBillingFingerprint(c)
	}
}

func buildUsageBillingFingerprint(c *UsageBillingCommand) string {
	if c == nil {
		return ""
	}
	raw := fmt.Sprintf(
		"%d|%d|%d|%s|%s|%s|%s|%d|%d|%d|%d|%d|%d|%s|%d|%0.10f|%0.10f|%0.10f|%0.10f|%0.10f",
		c.UserID,
		c.AccountID,
		c.APIKeyID,
		strings.TrimSpace(c.AccountType),
		strings.TrimSpace(c.Model),
		strings.TrimSpace(c.ServiceTier),
		strings.TrimSpace(c.ReasoningEffort),
		c.BillingType,
		c.InputTokens,
		c.OutputTokens,
		c.CacheCreationTokens,
		c.CacheReadTokens,
		c.ImageCount,
		strings.TrimSpace(c.MediaType),
		valueOrZero(c.SubscriptionID),
		c.BalanceCost,
		c.SubscriptionCost,
		c.APIKeyQuotaCost,
		c.APIKeyRateLimitCost,
		c.AccountQuotaCost,
	)
	if c.EntitlementID != nil || c.EntitlementBalanceFallback {
		raw += fmt.Sprintf("|ent:%d|fallback:%t|overage:%t", valueOrZero(c.EntitlementID), c.EntitlementBalanceFallback, c.AllowEntitlementOverage)
	}
	if payloadHash := strings.TrimSpace(c.RequestPayloadHash); payloadHash != "" {
		raw += "|" + payloadHash
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func HashUsageRequestPayload(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func valueOrZero(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// AccountQuotaState holds the post-increment quota state returned by the DB transaction.
// All values are post-update (i.e., already include the increment).
type AccountQuotaState struct {
	TotalUsed   float64
	TotalLimit  float64
	DailyUsed   float64
	DailyLimit  float64
	WeeklyUsed  float64
	WeeklyLimit float64
}

type UsageBillingApplyResult struct {
	Applied              bool
	APIKeyQuotaExhausted bool
	NewBalance           *float64           // post-deduction balance (nil = no balance deduction)
	BalanceOverdrafted   bool               // true when the sufficient-balance guard missed and debt was still recorded
	QuotaState           *AccountQuotaState // post-increment quota state (nil = no quota increment)
	SubscriptionVersion  int64              // post-increment subscription updated_at in Unix microseconds (0 = no subscription increment)
	EntitlementVersion   int64              // post-increment entitlement updated_at in Unix microseconds (0 = no entitlement increment)
}

// BatchImageBalanceHoldCommand describes an idempotent balance hold operation.
type BatchImageBalanceHoldCommand struct {
	RequestID                  string
	APIKeyID                   int64
	RequestFingerprint         string
	RequestPayloadHash         string
	UserID                     int64
	BatchID                    string
	GroupID                    *int64
	SubscriptionID             *int64
	EntitlementID              *int64
	BillingType                int8
	BillingSource              string
	EntitlementBalanceFallback bool
	HeldDailyWindowStart       *time.Time
	HeldWeeklyWindowStart      *time.Time
	HeldMonthlyWindowStart     *time.Time
	HoldAmount                 float64
	ActualAmount               float64
}

func (c *BatchImageBalanceHoldCommand) Normalize() {
	if c == nil {
		return
	}
	c.RequestID = strings.TrimSpace(c.RequestID)
	c.BatchID = strings.TrimSpace(c.BatchID)
	c.BillingSource = strings.TrimSpace(c.BillingSource)
	if !IsValidUsageBillingSource(c.BillingSource) {
		c.BillingSource = ResolveUsageBillingSource(c.BillingType, c.SubscriptionID, c.EntitlementID, false)
	}
	if strings.TrimSpace(c.RequestFingerprint) == "" {
		c.RequestFingerprint = buildBatchImageBalanceHoldFingerprint(c)
	}
}

func buildBatchImageBalanceHoldFingerprint(c *BatchImageBalanceHoldCommand) string {
	if c == nil {
		return ""
	}
	raw := fmt.Sprintf(
		"%d|%d|%s|group:%d|subscription:%d|entitlement:%d|type:%d|source:%s|fallback:%t|daily:%s|weekly:%s|monthly:%s|%0.10f|%0.10f",
		c.UserID,
		c.APIKeyID,
		strings.TrimSpace(c.BatchID),
		valueOrZero(c.GroupID),
		valueOrZero(c.SubscriptionID),
		valueOrZero(c.EntitlementID),
		c.BillingType,
		strings.TrimSpace(c.BillingSource),
		c.EntitlementBalanceFallback,
		batchImageBillingWindowFingerprint(c.HeldDailyWindowStart),
		batchImageBillingWindowFingerprint(c.HeldWeeklyWindowStart),
		batchImageBillingWindowFingerprint(c.HeldMonthlyWindowStart),
		c.HoldAmount,
		c.ActualAmount,
	)
	if payloadHash := strings.TrimSpace(c.RequestPayloadHash); payloadHash != "" {
		raw += "|" + payloadHash
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func batchImageBillingWindowFingerprint(windowStart *time.Time) string {
	if windowStart == nil {
		return ""
	}
	return windowStart.UTC().Format(time.RFC3339Nano)
}

type BatchImageBalanceHoldResult struct {
	Applied                bool
	NewBalance             *float64
	FrozenBalance          *float64
	BillingSource          string
	HeldDailyWindowStart   *time.Time
	HeldWeeklyWindowStart  *time.Time
	HeldMonthlyWindowStart *time.Time
}

type UsageBillingRepository interface {
	Apply(ctx context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error)
	ReserveBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error)
	CaptureBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error)
	ReleaseBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error)
}
