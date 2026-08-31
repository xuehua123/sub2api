package service

import (
	"context"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type apiKeyGroupAccessPolicyContextKey struct{}

type apiKeyGroupAccessPolicy struct {
	user         *User
	accessSource string
	entitlement  *SubscriptionEntitlement
	subscription *UserSubscription
}

// WithAPIKeyGroupAccessPolicy carries the authenticated API key's effective
// group authorization into gateway routing. This lets runtime fallbacks apply
// the same user-level policy as the group originally bound to the key.
func WithAPIKeyGroupAccessPolicy(ctx context.Context, apiKey *APIKey, entitlement *SubscriptionEntitlement, subscription *UserSubscription) context.Context {
	if ctx == nil || apiKey == nil || apiKey.User == nil {
		return ctx
	}
	return context.WithValue(ctx, apiKeyGroupAccessPolicyContextKey{}, apiKeyGroupAccessPolicy{
		user:         apiKey.User,
		accessSource: apiKey.EffectiveAccessSource(),
		entitlement:  entitlement,
		subscription: subscription,
	})
}

// IsAPIKeyResolvedGroupAllowed validates a group selected after authentication,
// such as a Claude Code fallback or an invalid-request fallback. Requests that
// do not originate from API-key middleware have no policy and keep the legacy
// behavior.
func IsAPIKeyResolvedGroupAllowed(ctx context.Context, group *Group) bool {
	if ctx == nil || group == nil {
		return true
	}
	policy, ok := ctx.Value(apiKeyGroupAccessPolicyContextKey{}).(apiKeyGroupAccessPolicy)
	if !ok || policy.user == nil || (!policy.user.RestrictToAllowedGroups && !policy.user.RestrictPublicGroups) {
		return true
	}
	if !policy.user.AllowsGroupByPolicy(group.ID, group.IsExclusive) {
		return false
	}
	if policy.entitlement != nil {
		for _, groupID := range entitlementEnabledGrantGroupIDs(policy.entitlement) {
			if groupID == group.ID {
				return true
			}
		}
		return false
	}
	if policy.subscription != nil {
		return policy.subscription.GroupID == group.ID
	}
	if policy.accessSource == APIKeyAccessSourceEntitlement {
		return false
	}
	return policy.user.CanBindGroup(group.ID, group.IsExclusive)
}

type User struct {
	ID                      int64
	Email                   string
	Username                string
	Notes                   string
	AvatarURL               string
	AvatarSource            string
	AvatarMIME              string
	AvatarByteSize          int
	AvatarSHA256            string
	PasswordHash            string
	Role                    string
	Balance                 float64
	FrozenBalance           float64
	Concurrency             int
	Status                  string
	DefaultChatAPIKeyID     *int64
	AllowedGroups           []int64
	RestrictToAllowedGroups bool
	RestrictPublicGroups    bool
	PaymentDisabled         bool
	TokenVersion            int64 // Incremented on password change to invalidate existing tokens
	// TokenVersionResolved indicates TokenVersion already contains the fingerprint-derived
	// value expected in JWT claims and refresh-token state.
	TokenVersionResolved bool
	SignupSource         string
	LastLoginAt          *time.Time
	LastActiveAt         *time.Time
	LastUsedAt           *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time // 非 nil 表示用户已软删除

	// GroupRates 用户专属分组倍率配置
	// map[groupID]rateMultiplier
	GroupRates map[int64]float64

	// TOTP 双因素认证字段
	TotpSecretEncrypted *string    // AES-256-GCM 加密的 TOTP 密钥
	TotpEnabled         bool       // 是否启用 TOTP
	TotpEnabledAt       *time.Time // TOTP 启用时间

	// Referral
	ReferralEnabled bool

	// 余额不足通知
	BalanceNotifyEnabled       bool
	BalanceNotifyThresholdType string // "fixed" (default) | "percentage"
	BalanceNotifyThreshold     *float64
	BalanceNotifyExtraEmails   []NotifyEmailEntry
	TotalRecharged             float64

	// RPMLimit 用户级每分钟请求数上限（0 = 不限制）。仅在所用分组未设置 rpm_limit
	// 且该 (用户, 分组) 无 rpm_override 时作为全局兜底生效，计数键 rpm:u:{userID}:{min}。
	RPMLimit int

	// UserGroupRPMOverride 来自 auth cache snapshot 的 (user, group) RPM 覆盖值。
	// nil = 该 API Key 对应的 (user, group) 无 override；非 nil 时 checkRPM 直接使用，
	// 避免每请求查 DB。字段不持久化到数据库。
	UserGroupRPMOverride *int

	APIKeys       []APIKey
	Subscriptions []UserSubscription
}

func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

func (u *User) IsActive() bool {
	return u.Status == StatusActive
}

// HasAllowedGroup reports whether groupID is present in the user's explicit
// user_allowed_groups allowlist.
func (u *User) HasAllowedGroup(groupID int64) bool {
	if u == nil {
		return false
	}
	for _, id := range u.AllowedGroups {
		if id == groupID {
			return true
		}
	}
	return false
}

// AllowsGroupType applies the user-level group-type policy without deciding
// how access is funded. In exclusive-only mode, balance allowlists,
// subscriptions, and entitlements may authorize access, but only to exclusive
// groups.
func (u *User) AllowsGroupType(isExclusive bool) bool {
	if u == nil {
		return false
	}
	return !u.RestrictToAllowedGroups || isExclusive
}

// AllowsGroupByPolicy applies both independent user-level access policies:
// restrict_to_allowed_groups rejects every public group, while
// restrict_public_groups keeps only explicitly allowlisted public groups.
// When both switches are enabled their intersection applies, so public groups
// remain unavailable even when they are present in AllowedGroups.
func (u *User) AllowsGroupByPolicy(groupID int64, isExclusive bool) bool {
	if u == nil || !u.AllowsGroupType(isExclusive) {
		return false
	}
	if !isExclusive && u.RestrictPublicGroups {
		return u.HasAllowedGroup(groupID)
	}
	return true
}

// CanBindGroup checks whether a user can bind to a given group.
// For standard groups:
//   - Public groups (non-exclusive): all users can bind unless one of the two
//     independent restriction policies denies the group
//   - Exclusive groups: only users with the group in AllowedGroups can bind
//   - Exclusive-only users: only explicitly granted exclusive groups are usable
func (u *User) CanBindGroup(groupID int64, isExclusive bool) bool {
	if u == nil {
		return false
	}
	if !u.AllowsGroupByPolicy(groupID, isExclusive) {
		return false
	}
	// 公开分组（非专属）：所有用户都可以绑定
	if !isExclusive {
		return true
	}
	// 专属分组：需要在 AllowedGroups 中
	return u.HasAllowedGroup(groupID)
}

func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	return nil
}

func (u *User) CheckPassword(password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil
}
