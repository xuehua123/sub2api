package service

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	ReferralCodeStatusActive   = "active"
	ReferralCodeStatusDisabled = "disabled"

	ReferralBindSourceLink          = "link"
	ReferralBindSourceManualInput   = "manual_input"
	ReferralBindSourceOAuthCallback = "oauth_callback"
	ReferralBindSourceAdminOverride = "admin_override"
)

var (
	ErrReferralDisabled                   = infraerrors.Forbidden("REFERRAL_DISABLED", "referral program is disabled")
	ErrReferralManualInputDisabled        = infraerrors.Forbidden("REFERRAL_MANUAL_INPUT_DISABLED", "manual referral binding is disabled")
	ErrReferralCodeNotFound               = infraerrors.NotFound("REFERRAL_CODE_NOT_FOUND", "referral code not found")
	ErrReferralCodeDisabled               = infraerrors.BadRequest("REFERRAL_CODE_DISABLED", "referral code is disabled")
	ErrReferralRelationNotFound           = infraerrors.NotFound("REFERRAL_RELATION_NOT_FOUND", "referral relation not found")
	ErrReferralAlreadyBound               = infraerrors.Conflict("REFERRAL_ALREADY_BOUND", "user is already bound to a referrer")
	ErrReferralSelfBind                   = infraerrors.BadRequest("REFERRAL_SELF_BIND", "cannot bind your own referral code")
	ErrReferralCycleDetected              = infraerrors.BadRequest("REFERRAL_CYCLE_DETECTED", "referral cycle detected")
	ErrReferralBindAfterPaymentNotAllowed = infraerrors.BadRequest("REFERRAL_BIND_AFTER_PAYMENT_NOT_ALLOWED", "referral code must be bound before the first paid recharge")
)

type ReferralCode struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Code      string    `json:"code"`
	Status    string    `json:"status"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ReferralRelation struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	ReferrerUserID int64      `json:"referrer_user_id"`
	BindSource     string     `json:"bind_source"`
	BindCode       *string    `json:"bind_code,omitempty"`
	LockedAt       *time.Time `json:"locked_at,omitempty"`
	Notes          *string    `json:"notes,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type ReferralRelationHistory struct {
	ID                int64     `json:"id"`
	UserID            int64     `json:"user_id"`
	OldReferrerUserID *int64    `json:"old_referrer_user_id,omitempty"`
	NewReferrerUserID *int64    `json:"new_referrer_user_id,omitempty"`
	OldBindCode       *string   `json:"old_bind_code,omitempty"`
	NewBindCode       *string   `json:"new_bind_code,omitempty"`
	ChangeSource      string    `json:"change_source"`
	ChangedBy         *int64    `json:"changed_by,omitempty"`
	Reason            *string   `json:"reason,omitempty"`
	MetadataJSON      *string   `json:"metadata_json,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type ReferralOverview struct {
	ReferralEnabled                 bool              `json:"referral_enabled"`
	AllowManualInput                bool              `json:"allow_manual_input"`
	BindBeforeFirstPaidOnly         bool              `json:"bind_before_first_paid_only"`
	ReferralWithdrawEnabled         bool              `json:"referral_withdraw_enabled"`
	ReferralCreditConversionEnabled bool              `json:"referral_credit_conversion_enabled"`
	SettlementCurrency              string            `json:"settlement_currency"`
	DefaultCode                     *ReferralCode     `json:"default_code,omitempty"`
	Relation                        *ReferralRelation `json:"relation,omitempty"`
	CanBind                         bool              `json:"can_bind"`
	HasPaidRecharge                 bool              `json:"has_paid_recharge"`
	ReferralWithdrawMethods         []string          `json:"withdraw_methods_enabled,omitempty"`
}

type ReferralCodePreview struct {
	ReferrerUserID      int64  `json:"referrer_user_id"`
	ReferrerUsername    string `json:"referrer_username"`
	ReferrerEmailMasked string `json:"referrer_email_masked"`
}

type BindReferralInput struct {
	UserID       int64
	Code         string
	BindSource   string
	ChangedBy    *int64
	Reason       *string
	MetadataJSON *string
	Notes        *string
}

type ReferralRepository interface {
	GetDefaultCodeByUserID(ctx context.Context, userID int64) (*ReferralCode, error)
	GetCodeByCode(ctx context.Context, code string) (*ReferralCode, error)
	CreateCode(ctx context.Context, code *ReferralCode) error
	GetRelationByUserID(ctx context.Context, userID int64) (*ReferralRelation, error)
	CreateRelation(ctx context.Context, relation *ReferralRelation) error
	CreateRelationHistory(ctx context.Context, history *ReferralRelationHistory) error
	HasPaidRecharge(ctx context.Context, userID int64) (bool, error)
}

type ReferralService struct {
	repo           ReferralRepository
	userRepo       UserRepository
	entClient      *dbent.Client
	settingService *SettingService
}

func NewReferralService(repo ReferralRepository, userRepo UserRepository, entClient *dbent.Client, settingService *SettingService) *ReferralService {
	return &ReferralService{
		repo:           repo,
		userRepo:       userRepo,
		entClient:      entClient,
		settingService: settingService,
	}
}

func (s *ReferralService) GetOverview(ctx context.Context, userID int64) (*ReferralOverview, error) {
	settings, err := s.getPublicSettings(ctx)
	if err != nil {
		return nil, err
	}

	userReferralEnabled, err := s.isReferralEnabledForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	var defaultCode *ReferralCode
	if userReferralEnabled {
		defaultCode, err = s.EnsureDefaultCode(ctx, userID)
		if err != nil {
			return nil, err
		}
	}

	relation, err := s.repo.GetRelationByUserID(ctx, userID)
	if err != nil && !errors.Is(err, ErrReferralRelationNotFound) {
		return nil, err
	}
	if errors.Is(err, ErrReferralRelationNotFound) {
		relation = nil
	}

	hasPaidRecharge, err := s.repo.HasPaidRecharge(ctx, userID)
	if err != nil {
		return nil, err
	}

	canBind := settings.ReferralAllowManualInput && relation == nil
	if settings.ReferralBindBeforeFirstPaidOnly && hasPaidRecharge {
		canBind = false
	}

	return &ReferralOverview{
		ReferralEnabled:                 userReferralEnabled,
		AllowManualInput:                settings.ReferralAllowManualInput,
		BindBeforeFirstPaidOnly:         settings.ReferralBindBeforeFirstPaidOnly,
		ReferralWithdrawEnabled:         settings.ReferralWithdrawEnabled,
		ReferralCreditConversionEnabled: settings.ReferralCreditConversionEnabled,
		SettlementCurrency:              settings.ReferralSettlementCurrency,
		DefaultCode:                     defaultCode,
		Relation:                        relation,
		CanBind:                         canBind,
		HasPaidRecharge:                 hasPaidRecharge,
		ReferralWithdrawMethods:         settings.ReferralWithdrawMethodsEnabled,
	}, nil
}

func (s *ReferralService) EnsureDefaultCode(ctx context.Context, userID int64) (*ReferralCode, error) {
	code, err := s.repo.GetDefaultCodeByUserID(ctx, userID)
	if err == nil {
		return code, nil
	}
	if !errors.Is(err, ErrReferralCodeNotFound) {
		return nil, err
	}

	for attempts := 0; attempts < 8; attempts++ {
		candidate, genErr := generateReferralCode()
		if genErr != nil {
			return nil, genErr
		}
		model := &ReferralCode{
			UserID:    userID,
			Code:      candidate,
			Status:    ReferralCodeStatusActive,
			IsDefault: true,
		}
		createErr := s.repo.CreateCode(ctx, model)
		if createErr == nil {
			return model, nil
		}
		// Only retry on unique constraint violations; other errors are fatal
		if !isUniqueConstraintError(createErr) {
			return nil, createErr
		}
		code, err = s.repo.GetDefaultCodeByUserID(ctx, userID)
		if err == nil {
			return code, nil
		}
		if !errors.Is(err, ErrReferralCodeNotFound) {
			return nil, err
		}
	}

	return nil, infraerrors.InternalServer("REFERRAL_CODE_CREATE_FAILED", "failed to create referral code")
}

func (s *ReferralService) BindReferralCode(ctx context.Context, input *BindReferralInput) (*ReferralRelation, error) {
	if input == nil {
		return nil, infraerrors.BadRequest("REFERRAL_BIND_INVALID", "invalid referral bind request")
	}
	settings, err := s.getPublicSettings(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.BindSource) == ReferralBindSourceManualInput && !settings.ReferralAllowManualInput {
		return nil, ErrReferralManualInputDisabled
	}

	codeValue := strings.ToUpper(strings.TrimSpace(input.Code))
	referralCode, err := s.ValidateReferralCode(ctx, codeValue)
	if err != nil {
		return nil, err
	}
	if referralCode.UserID == input.UserID {
		return nil, ErrReferralSelfBind
	}

	if err := s.ensureNoCycle(ctx, input.UserID, referralCode.UserID); err != nil {
		return nil, err
	}

	relation := &ReferralRelation{
		UserID:         input.UserID,
		ReferrerUserID: referralCode.UserID,
		BindSource:     normalizeReferralBindSource(input.BindSource),
		BindCode:       referralStringPtr(codeValue),
		Notes:          input.Notes,
	}
	history := &ReferralRelationHistory{
		UserID:            input.UserID,
		NewReferrerUserID: referralInt64Ptr(referralCode.UserID),
		NewBindCode:       referralStringPtr(codeValue),
		ChangeSource:      normalizeReferralBindSource(input.BindSource),
		ChangedBy:         input.ChangedBy,
		Reason:            input.Reason,
		MetadataJSON:      input.MetadataJSON,
	}

	apply := func(txCtx context.Context) error {
		existing, err := s.repo.GetRelationByUserID(txCtx, input.UserID)
		if err == nil && existing != nil {
			return ErrReferralAlreadyBound
		}
		if err != nil && !errors.Is(err, ErrReferralRelationNotFound) {
			return err
		}

		if settings.ReferralBindBeforeFirstPaidOnly {
			hasPaidRecharge, paidErr := s.repo.HasPaidRecharge(txCtx, input.UserID)
			if paidErr != nil {
				return paidErr
			}
			if hasPaidRecharge {
				return ErrReferralBindAfterPaymentNotAllowed
			}
		}

		if err := s.repo.CreateRelation(txCtx, relation); err != nil {
			return err
		}
		return s.repo.CreateRelationHistory(txCtx, history)
	}

	if s.entClient == nil || dbent.TxFromContext(ctx) != nil {
		if err := apply(ctx); err != nil {
			return nil, err
		}
		return relation, nil
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if err := apply(dbent.NewTxContext(ctx, tx)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return relation, nil
}

func (s *ReferralService) BindReferralCodeForNewUser(ctx context.Context, userID int64, code string, bindSource string) error {
	code = strings.TrimSpace(code)
	if code == "" || s == nil {
		return nil
	}
	_, err := s.BindReferralCode(ctx, &BindReferralInput{
		UserID:     userID,
		Code:       code,
		BindSource: normalizeReferralBindSource(bindSource),
	})
	return err
}

func (s *ReferralService) ValidateReferralCode(ctx context.Context, code string) (*ReferralCode, error) {
	codeValue := strings.ToUpper(strings.TrimSpace(code))
	referralCode, err := s.repo.GetCodeByCode(ctx, codeValue)
	if err != nil {
		return nil, err
	}

	// Check if the code owner has referral enabled (global or per-user)
	ownerEnabled, err := s.isReferralEnabledForUser(ctx, referralCode.UserID)
	if err != nil {
		return nil, err
	}
	if !ownerEnabled {
		return nil, ErrReferralDisabled
	}

	if referralCode.Status != ReferralCodeStatusActive {
		return nil, ErrReferralCodeDisabled
	}
	return referralCode, nil
}

func (s *ReferralService) PreviewReferralCode(ctx context.Context, code string) (*ReferralCodePreview, error) {
	referralCode, err := s.ValidateReferralCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if s.userRepo == nil {
		return &ReferralCodePreview{
			ReferrerUserID: referralCode.UserID,
		}, nil
	}
	user, err := s.userRepo.GetByID(ctx, referralCode.UserID)
	if err != nil {
		return nil, err
	}
	return &ReferralCodePreview{
		ReferrerUserID:      referralCode.UserID,
		ReferrerUsername:    user.Username,
		ReferrerEmailMasked: MaskEmail(user.Email),
	}, nil
}

func (s *ReferralService) getPublicSettings(ctx context.Context) (*PublicSettings, error) {
	if s.settingService == nil {
		return &PublicSettings{
			ReferralEnabled:                 false,
			ReferralAllowManualInput:        false,
			ReferralBindBeforeFirstPaidOnly: true,
			ReferralSettlementCurrency:      ReferralSettlementCurrencyCNY,
		}, nil
	}
	return s.settingService.GetPublicSettings(ctx)
}

// isReferralEnabledForUser returns true if referral is enabled for the given user,
// either via the global setting or the per-user override.
func (s *ReferralService) isReferralEnabledForUser(ctx context.Context, userID int64) (bool, error) {
	settings, err := s.getPublicSettings(ctx)
	if err != nil {
		return false, err
	}
	if settings.ReferralEnabled {
		return true, nil
	}
	if s.userRepo == nil {
		return false, nil
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	return user.ReferralEnabled, nil
}

func (s *ReferralService) ensureNoCycle(ctx context.Context, userID int64, referrerUserID int64) error {
	const maxDepth = 100
	visited := map[int64]struct{}{}
	current := referrerUserID
	for depth := 0; current > 0 && depth < maxDepth; depth++ {
		if current == userID {
			return ErrReferralCycleDetected
		}
		if _, ok := visited[current]; ok {
			return ErrReferralCycleDetected
		}
		visited[current] = struct{}{}

		relation, err := s.repo.GetRelationByUserID(ctx, current)
		if errors.Is(err, ErrReferralRelationNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		current = relation.ReferrerUserID
	}
	if current > 0 {
		return infraerrors.BadRequest("REFERRAL_CHAIN_TOO_DEEP", "referral chain exceeds maximum depth")
	}
	return nil
}

func normalizeReferralBindSource(raw string) string {
	switch strings.TrimSpace(raw) {
	case ReferralBindSourceManualInput:
		return ReferralBindSourceManualInput
	case ReferralBindSourceOAuthCallback:
		return ReferralBindSourceOAuthCallback
	case ReferralBindSourceAdminOverride:
		return ReferralBindSourceAdminOverride
	default:
		return ReferralBindSourceLink
	}
}

func generateReferralCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	const size = 8

	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	out := make([]byte, size)
	for i := 0; i < size; i++ {
		out[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(out), nil
}

func referralInt64Ptr(value int64) *int64 {
	return &value
}

func referralStringPtr(value string) *string {
	return &value
}

// isUniqueConstraintError detects unique constraint violations across DB drivers.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate entry")
}
