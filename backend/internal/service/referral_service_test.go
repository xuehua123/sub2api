//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type referralRepoStub struct {
	codesByUser     map[int64]*ReferralCode
	codesByCode     map[string]*ReferralCode
	relationsByUser map[int64]*ReferralRelation
	relationHistory []ReferralRelationHistory
	paidUsers       map[int64]bool
	nextCodeID      int64
	nextRelationID  int64
	nextHistoryID   int64
}

type referralRepoConcurrentDefaultStub struct {
	*referralRepoStub
	getCalls int
}

func (s *referralRepoConcurrentDefaultStub) GetDefaultCodeByUserID(ctx context.Context, userID int64) (*ReferralCode, error) {
	s.getCalls++
	if s.getCalls == 1 {
		return nil, ErrReferralCodeNotFound
	}
	return s.referralRepoStub.GetDefaultCodeByUserID(ctx, userID)
}

func (s *referralRepoConcurrentDefaultStub) CreateCode(ctx context.Context, code *ReferralCode) error {
	return errors.New("duplicate key value violates unique constraint referral_codes_user_id_is_default_key")
}

func newReferralRepoStub() *referralRepoStub {
	return &referralRepoStub{
		codesByUser:     make(map[int64]*ReferralCode),
		codesByCode:     make(map[string]*ReferralCode),
		relationsByUser: make(map[int64]*ReferralRelation),
		paidUsers:       make(map[int64]bool),
		nextCodeID:      1,
		nextRelationID:  1,
		nextHistoryID:   1,
	}
}

func (s *referralRepoStub) GetDefaultCodeByUserID(ctx context.Context, userID int64) (*ReferralCode, error) {
	if code, ok := s.codesByUser[userID]; ok {
		cloned := *code
		return &cloned, nil
	}
	return nil, ErrReferralCodeNotFound
}

func (s *referralRepoStub) GetCodeByCode(ctx context.Context, code string) (*ReferralCode, error) {
	if value, ok := s.codesByCode[code]; ok {
		cloned := *value
		return &cloned, nil
	}
	return nil, ErrReferralCodeNotFound
}

func (s *referralRepoStub) CreateCode(ctx context.Context, code *ReferralCode) error {
	if code.ID == 0 {
		code.ID = s.nextCodeID
		s.nextCodeID++
	}
	if code.CreatedAt.IsZero() {
		code.CreatedAt = time.Now()
	}
	if code.UpdatedAt.IsZero() {
		code.UpdatedAt = code.CreatedAt
	}
	cloned := *code
	s.codesByUser[code.UserID] = &cloned
	s.codesByCode[code.Code] = &cloned
	return nil
}

func (s *referralRepoStub) GetRelationByUserID(ctx context.Context, userID int64) (*ReferralRelation, error) {
	if relation, ok := s.relationsByUser[userID]; ok {
		cloned := *relation
		return &cloned, nil
	}
	return nil, ErrReferralRelationNotFound
}

func (s *referralRepoStub) CreateRelation(ctx context.Context, relation *ReferralRelation) error {
	if relation.ID == 0 {
		relation.ID = s.nextRelationID
		s.nextRelationID++
	}
	if relation.CreatedAt.IsZero() {
		relation.CreatedAt = time.Now()
	}
	if relation.UpdatedAt.IsZero() {
		relation.UpdatedAt = relation.CreatedAt
	}
	cloned := *relation
	s.relationsByUser[relation.UserID] = &cloned
	return nil
}

func (s *referralRepoStub) CreateRelationHistory(ctx context.Context, history *ReferralRelationHistory) error {
	if history.ID == 0 {
		history.ID = s.nextHistoryID
		s.nextHistoryID++
	}
	if history.CreatedAt.IsZero() {
		history.CreatedAt = time.Now()
	}
	cloned := *history
	s.relationHistory = append(s.relationHistory, cloned)
	return nil
}

func (s *referralRepoStub) HasPaidRecharge(ctx context.Context, userID int64) (bool, error) {
	return s.paidUsers[userID], nil
}

func newReferralServiceForTest(userRepo UserRepository, repo ReferralRepository, settings map[string]string) *ReferralService {
	cfg := &config.Config{
		Default: config.DefaultConfig{
			UserBalance:     3.5,
			UserConcurrency: 2,
		},
	}
	return NewReferralService(repo, userRepo, nil, NewSettingService(&settingRepoStub{values: settings}, cfg))
}

func TestReferralService_GetOverview_CreatesDefaultCodeWhenMissing(t *testing.T) {
	repo := newReferralRepoStub()
	userRepo := &userRepoStub{}
	svc := newReferralServiceForTest(userRepo, repo, map[string]string{
		SettingKeyReferralEnabled:          "true",
		SettingKeyReferralAllowManualInput: "true",
	})

	overview, err := svc.GetOverview(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, overview.DefaultCode)
	require.Equal(t, int64(7), overview.DefaultCode.UserID)
	require.NotEmpty(t, overview.DefaultCode.Code)
}

func TestReferralService_EnsureDefaultCode_ReturnsExistingCodeAfterConcurrentCreateConflict(t *testing.T) {
	baseRepo := newReferralRepoStub()
	baseRepo.codesByUser[7] = &ReferralCode{
		ID:        42,
		UserID:    7,
		Code:      "EXISTING",
		Status:    ReferralCodeStatusActive,
		IsDefault: true,
	}
	repo := &referralRepoConcurrentDefaultStub{referralRepoStub: baseRepo}
	svc := newReferralServiceForTest(&userRepoStub{}, repo, map[string]string{
		SettingKeyReferralEnabled:          "true",
		SettingKeyReferralAllowManualInput: "true",
	})

	code, err := svc.EnsureDefaultCode(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "EXISTING", code.Code)
	require.GreaterOrEqual(t, repo.getCalls, 2)
}

func TestReferralService_GetOverview_AllowsBindingWhenManualInputEnabledEvenIfUserCannotInvite(t *testing.T) {
	repo := newReferralRepoStub()
	userRepo := &userRepoStub{user: &User{ID: 7, Email: "invitee@example.com", Username: "invitee"}}
	svc := newReferralServiceForTest(userRepo, repo, map[string]string{
		SettingKeyReferralEnabled:          "false",
		SettingKeyReferralAllowManualInput: "true",
	})

	overview, err := svc.GetOverview(context.Background(), 7)
	require.NoError(t, err)
	require.False(t, overview.ReferralEnabled)
	require.True(t, overview.AllowManualInput)
	require.True(t, overview.CanBind)
}

func TestReferralService_BindReferralCode_CreatesRelationAndHistory(t *testing.T) {
	repo := newReferralRepoStub()
	repo.codesByCode["REF123"] = &ReferralCode{
		ID:        1,
		UserID:    99,
		Code:      "REF123",
		Status:    ReferralCodeStatusActive,
		IsDefault: true,
	}
	userRepo := &userRepoStub{}
	svc := newReferralServiceForTest(userRepo, repo, map[string]string{
		SettingKeyReferralEnabled:          "true",
		SettingKeyReferralAllowManualInput: "true",
	})

	relation, err := svc.BindReferralCode(context.Background(), &BindReferralInput{
		UserID:     7,
		Code:       "REF123",
		BindSource: ReferralBindSourceManualInput,
	})
	require.NoError(t, err)
	require.Equal(t, int64(99), relation.ReferrerUserID)
	require.Len(t, repo.relationHistory, 1)
	require.Equal(t, int64(7), repo.relationHistory[0].UserID)
	require.NotNil(t, repo.relationHistory[0].NewReferrerUserID)
	require.Equal(t, int64(99), *repo.relationHistory[0].NewReferrerUserID)
}

func TestReferralService_BindReferralCode_RejectsSelfInvite(t *testing.T) {
	repo := newReferralRepoStub()
	repo.codesByCode["SELF1"] = &ReferralCode{
		ID:        1,
		UserID:    7,
		Code:      "SELF1",
		Status:    ReferralCodeStatusActive,
		IsDefault: true,
	}
	svc := newReferralServiceForTest(&userRepoStub{}, repo, map[string]string{
		SettingKeyReferralEnabled:          "true",
		SettingKeyReferralAllowManualInput: "true",
	})

	_, err := svc.BindReferralCode(context.Background(), &BindReferralInput{
		UserID:     7,
		Code:       "SELF1",
		BindSource: ReferralBindSourceManualInput,
	})
	require.ErrorIs(t, err, ErrReferralSelfBind)
}

func TestReferralService_BindReferralCode_RejectsWhenAlreadyBound(t *testing.T) {
	repo := newReferralRepoStub()
	repo.codesByCode["REF123"] = &ReferralCode{
		ID:        1,
		UserID:    99,
		Code:      "REF123",
		Status:    ReferralCodeStatusActive,
		IsDefault: true,
	}
	repo.relationsByUser[7] = &ReferralRelation{
		ID:             1,
		UserID:         7,
		ReferrerUserID: 88,
		BindSource:     ReferralBindSourceLink,
	}
	svc := newReferralServiceForTest(&userRepoStub{}, repo, map[string]string{
		SettingKeyReferralEnabled:          "true",
		SettingKeyReferralAllowManualInput: "true",
	})

	_, err := svc.BindReferralCode(context.Background(), &BindReferralInput{
		UserID:     7,
		Code:       "REF123",
		BindSource: ReferralBindSourceManualInput,
	})
	require.ErrorIs(t, err, ErrReferralAlreadyBound)
}

func TestReferralService_BindReferralCode_RejectsAfterPaidRechargeWhenConfigured(t *testing.T) {
	repo := newReferralRepoStub()
	repo.paidUsers[7] = true
	repo.codesByCode["REF123"] = &ReferralCode{
		ID:        1,
		UserID:    99,
		Code:      "REF123",
		Status:    ReferralCodeStatusActive,
		IsDefault: true,
	}
	svc := newReferralServiceForTest(&userRepoStub{}, repo, map[string]string{
		SettingKeyReferralEnabled:                 "true",
		SettingKeyReferralAllowManualInput:        "true",
		SettingKeyReferralBindBeforeFirstPaidOnly: "true",
	})

	_, err := svc.BindReferralCode(context.Background(), &BindReferralInput{
		UserID:     7,
		Code:       "REF123",
		BindSource: ReferralBindSourceManualInput,
	})
	require.ErrorIs(t, err, ErrReferralBindAfterPaymentNotAllowed)
}

func TestReferralService_GetOverview_DoesNotCreateDefaultCodeWhenReferralDisabledForUser(t *testing.T) {
	repo := newReferralRepoStub()
	userRepo := &userRepoStub{user: &User{ID: 7, Email: "invitee@example.com", Username: "invitee", ReferralEnabled: false}}
	svc := newReferralServiceForTest(userRepo, repo, map[string]string{
		SettingKeyReferralEnabled:          "false",
		SettingKeyReferralAllowManualInput: "true",
	})

	overview, err := svc.GetOverview(context.Background(), 7)
	require.NoError(t, err)
	require.Nil(t, overview.DefaultCode)
	_, exists := repo.codesByUser[7]
	require.False(t, exists)
}
