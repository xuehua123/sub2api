package domain

import (
	"net/mail"
	"regexp"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SupportIssueStatusOpen       = "open"
	SupportIssueStatusNeedsInfo  = "needs_info"
	SupportIssueStatusInProgress = "in_progress"
	SupportIssueStatusResolved   = "resolved"
	SupportIssueStatusClosed     = "closed"
)

const (
	SupportIssueCategoryLogin            = "login"
	SupportIssueCategoryPayment          = "payment"
	SupportIssueCategoryAPICall          = "api_call"
	SupportIssueCategoryModelUnavailable = "model_unavailable"
	SupportIssueCategoryAPIKey           = "api_key"
	SupportIssueCategoryBalance          = "balance"
	SupportIssueCategorySubscription     = "subscription"
	SupportIssueCategoryChannel          = "channel"
	SupportIssueCategoryOther            = "other"
)

const (
	SupportIssueSeverityBlocked      = "blocked"
	SupportIssueSeverityPartial      = "partial"
	SupportIssueSeverityIntermittent = "intermittent"
	SupportIssueSeverityQuestion     = "question"
)

const (
	SupportIssueScreenshotLanguageZH      = "zh"
	SupportIssueScreenshotLanguageEN      = "en"
	SupportIssueScreenshotLanguageMixed   = "mixed"
	SupportIssueScreenshotLanguageUnknown = "unknown"
)

const (
	SupportIssueEventCreated          = "created"
	SupportIssueEventCommented        = "commented"
	SupportIssueEventStatusChanged    = "status_changed"
	SupportIssueEventLocked           = "locked"
	SupportIssueEventReopened         = "reopened"
	SupportIssueEventCommentHidden    = "comment_hidden"
	SupportIssueEventAttachmentHidden = "attachment_hidden"
)

const (
	SupportIssueTransitionActorAdmin    = "admin"
	SupportIssueTransitionActorReporter = "reporter"
	SupportIssueTransitionActorUser     = "user"
)

const (
	SupportIssueErrorNotFound              = "SUPPORT_ISSUE_NOT_FOUND"
	SupportIssueErrorLocked                = "SUPPORT_ISSUE_LOCKED"
	SupportIssueErrorInvalidStatus         = "SUPPORT_ISSUE_INVALID_STATUS"
	SupportIssueErrorInvalidTransition     = "SUPPORT_ISSUE_INVALID_TRANSITION"
	SupportIssueErrorPermissionDenied      = "SUPPORT_ISSUE_PERMISSION_DENIED"
	SupportIssueErrorTitleInvalid          = "SUPPORT_ISSUE_TITLE_INVALID"
	SupportIssueErrorDescriptionInvalid    = "SUPPORT_ISSUE_DESCRIPTION_INVALID"
	SupportIssueErrorEmailInvalid          = "SUPPORT_ISSUE_EMAIL_INVALID"
	SupportIssueErrorScreenshotTextInvalid = "SUPPORT_ISSUE_SCREENSHOT_TEXT_REQUIRED"
	SupportIssueErrorSearchInvalid         = "SUPPORT_ISSUE_SEARCH_INVALID"
	SupportIssueErrorAttachmentInvalidType = "SUPPORT_ISSUE_ATTACHMENT_INVALID_TYPE"
	SupportIssueErrorAttachmentTooLarge    = "SUPPORT_ISSUE_ATTACHMENT_TOO_LARGE"
)

var (
	ErrSupportIssueNotFound              = infraerrors.NotFound(SupportIssueErrorNotFound, "support issue not found")
	ErrSupportIssueLocked                = infraerrors.Conflict(SupportIssueErrorLocked, "support issue is locked")
	ErrSupportIssueInvalidStatus         = infraerrors.BadRequest(SupportIssueErrorInvalidStatus, "invalid support issue status")
	ErrSupportIssueInvalidTransition     = infraerrors.BadRequest(SupportIssueErrorInvalidTransition, "invalid support issue status transition")
	ErrSupportIssuePermissionDenied      = infraerrors.Forbidden(SupportIssueErrorPermissionDenied, "support issue permission denied")
	ErrSupportIssueTitleInvalid          = infraerrors.BadRequest(SupportIssueErrorTitleInvalid, "support issue title must be 4-160 characters")
	ErrSupportIssueDescriptionInvalid    = infraerrors.BadRequest(SupportIssueErrorDescriptionInvalid, "support issue description must be 10-12000 characters")
	ErrSupportIssueEmailInvalid          = infraerrors.BadRequest(SupportIssueErrorEmailInvalid, "invalid support issue account email")
	ErrSupportIssueScreenshotTextInvalid = infraerrors.BadRequest(SupportIssueErrorScreenshotTextInvalid, "support issue screenshot text must be 2-8000 characters")
	ErrSupportIssueSearchInvalid         = infraerrors.BadRequest(SupportIssueErrorSearchInvalid, "invalid support issue search query")
	ErrSupportIssueAttachmentInvalidType = infraerrors.BadRequest(SupportIssueErrorAttachmentInvalidType, "invalid support issue attachment type")
	ErrSupportIssueAttachmentTooLarge    = infraerrors.BadRequest(SupportIssueErrorAttachmentTooLarge, "support issue attachment is too large")
)

var supportIssueAPIKeySuffixPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{4,16}$`)

type SupportIssueBaseFields struct {
	Title              string
	Description        string
	AccountEmail       string
	OccurredAt         time.Time
	ScreenshotText     string
	Category           string
	Severity           string
	Status             string
	ScreenshotLanguage string
	HTTPStatus         *int
	APIKeySuffix       string
}

type NormalizedSupportIssueBaseFields struct {
	Title                  string
	Description            string
	AccountEmail           string
	AccountEmailNormalized string
	AccountEmailMasked     string
	OccurredAt             time.Time
	ScreenshotText         string
	Category               string
	Severity               string
	Status                 string
	ScreenshotLanguage     string
	HTTPStatus             *int
	APIKeySuffix           string
}

func NewSupportIssueSearchInvalidError(message string) error {
	return infraerrors.BadRequest(SupportIssueErrorSearchInvalid, message)
}

func ValidateSupportIssueBaseFields(input SupportIssueBaseFields, now time.Time) (NormalizedSupportIssueBaseFields, error) {
	title, err := ValidateSupportIssueTitle(input.Title)
	if err != nil {
		return NormalizedSupportIssueBaseFields{}, err
	}

	description, err := ValidateSupportIssueDescription(input.Description)
	if err != nil {
		return NormalizedSupportIssueBaseFields{}, err
	}

	email, err := NormalizeSupportIssueEmail(input.AccountEmail)
	if err != nil {
		return NormalizedSupportIssueBaseFields{}, err
	}

	maskedEmail, err := MaskSupportIssueEmail(email)
	if err != nil {
		return NormalizedSupportIssueBaseFields{}, err
	}

	if err := ValidateSupportIssueOccurredAt(input.OccurredAt, now); err != nil {
		return NormalizedSupportIssueBaseFields{}, err
	}

	screenshotText, err := ValidateSupportIssueScreenshotText(input.ScreenshotText)
	if err != nil {
		return NormalizedSupportIssueBaseFields{}, err
	}

	category, err := ValidateSupportIssueCategory(input.Category)
	if err != nil {
		return NormalizedSupportIssueBaseFields{}, err
	}

	severity, err := ValidateSupportIssueSeverity(input.Severity)
	if err != nil {
		return NormalizedSupportIssueBaseFields{}, err
	}

	status, err := ValidateSupportIssueStatus(input.Status)
	if err != nil {
		return NormalizedSupportIssueBaseFields{}, err
	}

	language, err := ValidateSupportIssueScreenshotLanguage(input.ScreenshotLanguage)
	if err != nil {
		return NormalizedSupportIssueBaseFields{}, err
	}

	if err := ValidateSupportIssueHTTPStatus(input.HTTPStatus); err != nil {
		return NormalizedSupportIssueBaseFields{}, err
	}

	apiKeySuffix, err := NormalizeSupportIssueAPIKeySuffix(input.APIKeySuffix)
	if err != nil {
		return NormalizedSupportIssueBaseFields{}, err
	}

	return NormalizedSupportIssueBaseFields{
		Title:                  title,
		Description:            description,
		AccountEmail:           email,
		AccountEmailNormalized: email,
		AccountEmailMasked:     maskedEmail,
		OccurredAt:             input.OccurredAt,
		ScreenshotText:         screenshotText,
		Category:               category,
		Severity:               severity,
		Status:                 status,
		ScreenshotLanguage:     language,
		HTTPStatus:             input.HTTPStatus,
		APIKeySuffix:           apiKeySuffix,
	}, nil
}

func ValidateSupportIssueTitle(title string) (string, error) {
	trimmed := strings.TrimSpace(title)
	if len([]rune(trimmed)) < 4 || len([]rune(trimmed)) > 160 {
		return "", ErrSupportIssueTitleInvalid
	}
	return trimmed, nil
}

func ValidateSupportIssueDescription(description string) (string, error) {
	trimmed := strings.TrimSpace(description)
	if len([]rune(trimmed)) < 10 || len([]rune(trimmed)) > 12000 {
		return "", ErrSupportIssueDescriptionInvalid
	}
	return trimmed, nil
}

func NormalizeSupportIssueEmail(email string) (string, error) {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" || strings.ContainsAny(trimmed, "<>") {
		return "", ErrSupportIssueEmailInvalid
	}

	parsed, err := mail.ParseAddress(trimmed)
	if err != nil || parsed == nil || parsed.Address == "" || !strings.EqualFold(parsed.Address, trimmed) {
		return "", ErrSupportIssueEmailInvalid
	}

	normalized := strings.ToLower(parsed.Address)
	if strings.Count(normalized, "@") != 1 {
		return "", ErrSupportIssueEmailInvalid
	}
	local, domain, ok := strings.Cut(normalized, "@")
	if !ok || local == "" || domain == "" || !strings.Contains(domain, ".") {
		return "", ErrSupportIssueEmailInvalid
	}

	return normalized, nil
}

func MaskSupportIssueEmail(email string) (string, error) {
	normalized, err := NormalizeSupportIssueEmail(email)
	if err != nil {
		return "", err
	}

	local, domainPart, _ := strings.Cut(normalized, "@")
	first := []rune(local)[0]
	return string(first) + "***@" + domainPart, nil
}

func ValidateSupportIssueOccurredAt(occurredAt time.Time, now time.Time) error {
	if occurredAt.IsZero() {
		return infraerrors.BadRequest(SupportIssueErrorInvalidStatus, "support issue occurred_at is required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if occurredAt.After(now.AddDate(0, 0, 30)) {
		return infraerrors.BadRequest(SupportIssueErrorInvalidStatus, "support issue occurred_at cannot be more than 30 days in the future")
	}
	return nil
}

func ValidateSupportIssueScreenshotText(text string) (string, error) {
	trimmed := strings.TrimSpace(text)
	if len([]rune(trimmed)) < 2 || len([]rune(trimmed)) > 8000 {
		return "", ErrSupportIssueScreenshotTextInvalid
	}
	return trimmed, nil
}

func ValidateSupportIssueCategory(category string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(category))
	switch normalized {
	case SupportIssueCategoryLogin,
		SupportIssueCategoryPayment,
		SupportIssueCategoryAPICall,
		SupportIssueCategoryModelUnavailable,
		SupportIssueCategoryAPIKey,
		SupportIssueCategoryBalance,
		SupportIssueCategorySubscription,
		SupportIssueCategoryChannel,
		SupportIssueCategoryOther:
		return normalized, nil
	default:
		return "", infraerrors.BadRequest(SupportIssueErrorInvalidStatus, "invalid support issue category")
	}
}

func ValidateSupportIssueSeverity(severity string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(severity))
	switch normalized {
	case SupportIssueSeverityBlocked,
		SupportIssueSeverityPartial,
		SupportIssueSeverityIntermittent,
		SupportIssueSeverityQuestion:
		return normalized, nil
	default:
		return "", infraerrors.BadRequest(SupportIssueErrorInvalidStatus, "invalid support issue severity")
	}
}

func ValidateSupportIssueStatus(status string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch normalized {
	case SupportIssueStatusOpen,
		SupportIssueStatusNeedsInfo,
		SupportIssueStatusInProgress,
		SupportIssueStatusResolved,
		SupportIssueStatusClosed:
		return normalized, nil
	default:
		return "", ErrSupportIssueInvalidStatus
	}
}

func ValidateSupportIssueScreenshotLanguage(language string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(language))
	switch normalized {
	case SupportIssueScreenshotLanguageZH,
		SupportIssueScreenshotLanguageEN,
		SupportIssueScreenshotLanguageMixed,
		SupportIssueScreenshotLanguageUnknown:
		return normalized, nil
	default:
		return "", infraerrors.BadRequest(SupportIssueErrorInvalidStatus, "invalid support issue screenshot language")
	}
}

func ValidateSupportIssueHTTPStatus(status *int) error {
	if status == nil {
		return nil
	}
	if *status < 100 || *status > 599 {
		return infraerrors.BadRequest(SupportIssueErrorInvalidStatus, "support issue http status must be between 100 and 599")
	}
	return nil
}

func NormalizeSupportIssueAPIKeySuffix(suffix string) (string, error) {
	normalized := strings.Join(strings.Fields(suffix), "")
	if normalized == "" {
		return "", nil
	}
	if !supportIssueAPIKeySuffixPattern.MatchString(normalized) {
		return "", infraerrors.BadRequest(SupportIssueErrorInvalidStatus, "support issue api key suffix must be 4-16 safe characters")
	}
	return normalized, nil
}

func IsSupportIssueLockedStatus(status string) bool {
	normalized, err := ValidateSupportIssueStatus(status)
	if err != nil {
		return false
	}
	return normalized == SupportIssueStatusResolved || normalized == SupportIssueStatusClosed
}

func CanTransitionSupportIssueStatus(from string, to string, actor string) bool {
	return ValidateSupportIssueStatusTransition(from, to, actor) == nil
}

func ValidateSupportIssueStatusTransition(from string, to string, actor string) error {
	fromStatus, err := ValidateSupportIssueStatus(from)
	if err != nil {
		return err
	}
	toStatus, err := ValidateSupportIssueStatus(to)
	if err != nil {
		return err
	}

	actor = strings.ToLower(strings.TrimSpace(actor))
	if supportIssueStatusTransitionAllowed(fromStatus, toStatus, actor) {
		return nil
	}
	return ErrSupportIssueInvalidTransition
}

func supportIssueStatusTransitionAllowed(from string, to string, actor string) bool {
	switch from {
	case SupportIssueStatusOpen:
		switch to {
		case SupportIssueStatusNeedsInfo, SupportIssueStatusInProgress, SupportIssueStatusClosed:
			return actor == SupportIssueTransitionActorAdmin
		case SupportIssueStatusResolved:
			return actor == SupportIssueTransitionActorAdmin || actor == SupportIssueTransitionActorReporter
		}
	case SupportIssueStatusNeedsInfo:
		switch to {
		case SupportIssueStatusOpen, SupportIssueStatusResolved:
			return actor == SupportIssueTransitionActorAdmin || actor == SupportIssueTransitionActorReporter
		case SupportIssueStatusInProgress, SupportIssueStatusClosed:
			return actor == SupportIssueTransitionActorAdmin
		}
	case SupportIssueStatusInProgress:
		switch to {
		case SupportIssueStatusNeedsInfo, SupportIssueStatusClosed:
			return actor == SupportIssueTransitionActorAdmin
		case SupportIssueStatusResolved:
			return actor == SupportIssueTransitionActorAdmin || actor == SupportIssueTransitionActorReporter
		}
	case SupportIssueStatusResolved, SupportIssueStatusClosed:
		return to == SupportIssueStatusOpen && actor == SupportIssueTransitionActorAdmin
	}
	return false
}
