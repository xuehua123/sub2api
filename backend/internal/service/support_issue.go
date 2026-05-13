package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	SupportIssueStatusOpen       = domain.SupportIssueStatusOpen
	SupportIssueStatusNeedsInfo  = domain.SupportIssueStatusNeedsInfo
	SupportIssueStatusInProgress = domain.SupportIssueStatusInProgress
	SupportIssueStatusResolved   = domain.SupportIssueStatusResolved
	SupportIssueStatusClosed     = domain.SupportIssueStatusClosed
)

const (
	SupportIssueCategoryLogin            = domain.SupportIssueCategoryLogin
	SupportIssueCategoryPayment          = domain.SupportIssueCategoryPayment
	SupportIssueCategoryAPICall          = domain.SupportIssueCategoryAPICall
	SupportIssueCategoryModelUnavailable = domain.SupportIssueCategoryModelUnavailable
	SupportIssueCategoryAPIKey           = domain.SupportIssueCategoryAPIKey
	SupportIssueCategoryBalance          = domain.SupportIssueCategoryBalance
	SupportIssueCategorySubscription     = domain.SupportIssueCategorySubscription
	SupportIssueCategoryChannel          = domain.SupportIssueCategoryChannel
	SupportIssueCategoryOther            = domain.SupportIssueCategoryOther
)

const (
	SupportIssueSeverityBlocked      = domain.SupportIssueSeverityBlocked
	SupportIssueSeverityPartial      = domain.SupportIssueSeverityPartial
	SupportIssueSeverityIntermittent = domain.SupportIssueSeverityIntermittent
	SupportIssueSeverityQuestion     = domain.SupportIssueSeverityQuestion
)

const (
	SupportIssueScreenshotLanguageZH      = domain.SupportIssueScreenshotLanguageZH
	SupportIssueScreenshotLanguageEN      = domain.SupportIssueScreenshotLanguageEN
	SupportIssueScreenshotLanguageMixed   = domain.SupportIssueScreenshotLanguageMixed
	SupportIssueScreenshotLanguageUnknown = domain.SupportIssueScreenshotLanguageUnknown
)

const (
	SupportIssueEventCreated          = domain.SupportIssueEventCreated
	SupportIssueEventCommented        = domain.SupportIssueEventCommented
	SupportIssueEventStatusChanged    = domain.SupportIssueEventStatusChanged
	SupportIssueEventLocked           = domain.SupportIssueEventLocked
	SupportIssueEventReopened         = domain.SupportIssueEventReopened
	SupportIssueEventCommentHidden    = domain.SupportIssueEventCommentHidden
	SupportIssueEventAttachmentHidden = domain.SupportIssueEventAttachmentHidden
)

const (
	SupportIssueAttachmentVisibilityPublic = "public"
	SupportIssueAttachmentVisibilityHidden = "hidden"
)

const (
	SupportIssueDefaultHideReason      = "admin moderation"
	SupportIssueMaxCommentContentRunes = 8000
)

var (
	ErrSupportIssueNotFound              = domain.ErrSupportIssueNotFound
	ErrSupportIssueLocked                = domain.ErrSupportIssueLocked
	ErrSupportIssueInvalidStatus         = domain.ErrSupportIssueInvalidStatus
	ErrSupportIssueInvalidTransition     = domain.ErrSupportIssueInvalidTransition
	ErrSupportIssuePermissionDenied      = domain.ErrSupportIssuePermissionDenied
	ErrSupportIssueTitleInvalid          = domain.ErrSupportIssueTitleInvalid
	ErrSupportIssueDescriptionInvalid    = domain.ErrSupportIssueDescriptionInvalid
	ErrSupportIssueEmailInvalid          = domain.ErrSupportIssueEmailInvalid
	ErrSupportIssueScreenshotTextInvalid = domain.ErrSupportIssueScreenshotTextInvalid
	ErrSupportIssueSearchInvalid         = domain.ErrSupportIssueSearchInvalid
	ErrSupportIssueAttachmentInvalidType = domain.ErrSupportIssueAttachmentInvalidType
	ErrSupportIssueAttachmentTooLarge    = domain.ErrSupportIssueAttachmentTooLarge
	ErrSupportIssueInvalidInput          = infraerrors.BadRequest("SUPPORT_ISSUE_INPUT_INVALID", "invalid support issue input")
	ErrSupportIssueCommentNotFound       = infraerrors.NotFound("SUPPORT_ISSUE_COMMENT_NOT_FOUND", "support issue comment not found")
	ErrSupportIssueAttachmentNotFound    = infraerrors.NotFound("SUPPORT_ISSUE_ATTACHMENT_NOT_FOUND", "support issue attachment not found")
)

type SupportIssue struct {
	ID                     int64
	PublicID               string
	Title                  string
	Description            string
	AccountEmail           string
	AccountEmailNormalized string
	AccountEmailMasked     string
	OccurredAt             time.Time
	ScreenshotText         string
	ScreenshotLanguage     string
	Category               string
	Severity               string
	Status                 string
	ModelName              string
	ClientName             string
	HTTPStatus             *int
	ErrorCode              string
	APIKeySuffix           string
	CreatedByUserID        *int64
	ResolvedByUserID       *int64
	ResolvedAt             *time.Time
	LockedAt               *time.Time
	LastCommentAt          *time.Time
	CommentCount           int
	HiddenCommentCount     int
	AttachmentCount        int
	SearchText             string
	CreatedAt              time.Time
	UpdatedAt              time.Time
	Comments               []SupportIssueComment
	Attachments            []SupportIssueAttachment
	Events                 []SupportIssueEvent
}

type SupportIssueComment struct {
	ID             int64
	IssueID        int64
	AuthorUserID   *int64
	AuthorRole     string
	Content        string
	HiddenAt       *time.Time
	HiddenByUserID *int64
	HideReason     string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type SupportIssueAttachment struct {
	ID               int64
	IssueID          int64
	UploadedByUserID *int64
	FilePath         string
	FileURL          string
	FileName         string
	MimeType         string
	SizeBytes        int64
	OCRText          string
	Visibility       string
	HiddenAt         *time.Time
	HiddenByUserID   *int64
	CreatedAt        time.Time
}

type SupportIssueEvent struct {
	ID          int64
	IssueID     int64
	ActorUserID *int64
	EventType   string
	FromStatus  *string
	ToStatus    *string
	Metadata    map[string]any
	CreatedAt   time.Time
}

type CreateSupportIssueInput struct {
	Title                  string
	Description            string
	AccountEmail           string
	AccountEmailNormalized string
	AccountEmailMasked     string
	OccurredAt             time.Time
	ScreenshotText         string
	ScreenshotLanguage     string
	Category               string
	Severity               string
	Status                 string
	ModelName              string
	ClientName             string
	HTTPStatus             *int
	ErrorCode              string
	APIKeySuffix           string
	SearchText             string
	Attachments            []SupportIssueAttachment
}

type ListSupportIssueFilters struct {
	Status   string
	Category string
	Severity string
	HasImage *bool
}

type SearchSupportIssueQuery struct {
	Parsed        ParsedSupportIssueSearch
	Filters       ListSupportIssueFilters
	IncludeHidden bool
}

type AddSupportIssueCommentInput struct {
	IssueID      int64
	AuthorUserID int64
	AuthorRole   string
	Content      string
}

type UpdateSupportIssueStatusInput struct {
	IssueID      int64
	NextStatus   string
	ActorUserID  int64
	ActorIsAdmin bool
}

type HideSupportIssueCommentInput struct {
	IssueID        int64
	CommentID      int64
	HiddenByUserID int64
	HideReason     string
}

type HideSupportIssueAttachmentInput struct {
	IssueID        int64
	AttachmentID   int64
	HiddenByUserID int64
}

type SupportIssueActor struct {
	UserID  int64
	Email   string
	Role    string
	IsAdmin bool
}

type SupportIssueRepository interface {
	CreateIssue(ctx context.Context, issue *SupportIssue, attachments []SupportIssueAttachment, event SupportIssueEvent) error
	GetIssue(ctx context.Context, id int64, includeHidden bool) (*SupportIssue, error)
	ListIssues(ctx context.Context, params pagination.PaginationParams, filters ListSupportIssueFilters) ([]SupportIssue, *pagination.PaginationResult, error)
	SearchIssues(ctx context.Context, params pagination.PaginationParams, query SearchSupportIssueQuery) ([]SupportIssue, *pagination.PaginationResult, error)
	AddComment(ctx context.Context, comment *SupportIssueComment, event SupportIssueEvent) error
	UpdateStatus(ctx context.Context, issueID int64, nextStatus string, actorUserID int64, actorIsAdmin bool, event SupportIssueEvent) (*SupportIssue, error)
	HideComment(ctx context.Context, input HideSupportIssueCommentInput, event SupportIssueEvent) error
	HideAttachment(ctx context.Context, input HideSupportIssueAttachmentInput, event SupportIssueEvent) error
	ListEvents(ctx context.Context, issueID int64) ([]SupportIssueEvent, error)
}
