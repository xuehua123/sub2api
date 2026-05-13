package dto

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type CreateSupportIssueRequest struct {
	Title              string                          `json:"title" binding:"required"`
	Description        string                          `json:"description" binding:"required"`
	AccountEmail       string                          `json:"account_email" binding:"required"`
	OccurredAt         time.Time                       `json:"occurred_at" binding:"required"`
	ScreenshotText     string                          `json:"screenshot_text" binding:"required"`
	ScreenshotLanguage string                          `json:"screenshot_language" binding:"required"`
	Category           string                          `json:"category" binding:"required"`
	Severity           string                          `json:"severity" binding:"required"`
	ModelName          string                          `json:"model_name"`
	ClientName         string                          `json:"client_name"`
	HTTPStatus         *int                            `json:"http_status"`
	ErrorCode          string                          `json:"error_code"`
	APIKeySuffix       string                          `json:"api_key_suffix"`
	Attachments        []SupportIssueAttachmentRequest `json:"attachments"`
	AttachmentIDs      []int64                         `json:"attachment_ids"`
}

type SupportIssueAttachmentRequest struct {
	FileURL   string `json:"file_url"`
	FileName  string `json:"file_name"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	OCRText   string `json:"ocr_text"`
}

type AddSupportIssueCommentRequest struct {
	Content        string `json:"content" binding:"required"`
	RelatedIssueID *int64 `json:"related_issue_id"`
}

type UpdateSupportIssueStatusRequest struct {
	Status string `json:"status" binding:"required"`
	Reason string `json:"reason"`
}

type SupportIssueReasonRequest struct {
	Reason string `json:"reason"`
}

type PinSupportIssueRequest struct {
	Reason string `json:"reason"`
}

type MarkSupportIssueSolutionRequest struct {
	CommentID int64 `json:"comment_id" binding:"required"`
}

type SetRelatedSupportIssueRequest struct {
	RelatedIssueID int64  `json:"related_issue_id" binding:"required"`
	Reason         string `json:"reason"`
}

type SupportIssueReference struct {
	ID         int64      `json:"id"`
	PublicID   string     `json:"public_id"`
	Title      string     `json:"title"`
	Status     string     `json:"status"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

type PublicSupportIssue struct {
	ID                 int64                          `json:"id"`
	PublicID           string                         `json:"public_id"`
	Title              string                         `json:"title"`
	Description        string                         `json:"description"`
	AccountEmailMasked string                         `json:"account_email_masked"`
	OccurredAt         time.Time                      `json:"occurred_at"`
	ScreenshotText     string                         `json:"screenshot_text"`
	ScreenshotLanguage string                         `json:"screenshot_language"`
	Category           string                         `json:"category"`
	Severity           string                         `json:"severity"`
	Status             string                         `json:"status"`
	ModelName          string                         `json:"model_name,omitempty"`
	ClientName         string                         `json:"client_name,omitempty"`
	HTTPStatus         *int                           `json:"http_status,omitempty"`
	ErrorCode          string                         `json:"error_code,omitempty"`
	ResolvedAt         *time.Time                     `json:"resolved_at,omitempty"`
	LockedAt           *time.Time                     `json:"locked_at,omitempty"`
	PinnedAt           *time.Time                     `json:"pinned_at,omitempty"`
	SolutionCommentID  *int64                         `json:"solution_comment_id,omitempty"`
	RelatedIssueID     *int64                         `json:"related_issue_id,omitempty"`
	LastCommentAt      *time.Time                     `json:"last_comment_at,omitempty"`
	LastViewedAt       *time.Time                     `json:"last_viewed_at,omitempty"`
	CommentCount       int                            `json:"comment_count"`
	AttachmentCount    int                            `json:"attachment_count"`
	ViewCount          int                            `json:"view_count"`
	CreatedAt          time.Time                      `json:"created_at"`
	UpdatedAt          time.Time                      `json:"updated_at"`
	Comments           []PublicSupportIssueComment    `json:"comments,omitempty"`
	Attachments        []PublicSupportIssueAttachment `json:"attachments,omitempty"`
	SolutionComment    *PublicSupportIssueComment     `json:"solution_comment,omitempty"`
	RelatedIssue       *SupportIssueReference         `json:"related_issue,omitempty"`
}

type PublicSupportIssueComment struct {
	ID             int64                  `json:"id"`
	IssueID        int64                  `json:"issue_id"`
	AuthorRole     string                 `json:"author_role"`
	Content        string                 `json:"content"`
	RelatedIssueID *int64                 `json:"related_issue_id,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	RelatedIssue   *SupportIssueReference `json:"related_issue,omitempty"`
}

type PublicSupportIssueAttachment struct {
	ID         int64     `json:"id"`
	IssueID    int64     `json:"issue_id"`
	FileURL    string    `json:"file_url"`
	FileName   string    `json:"file_name"`
	MimeType   string    `json:"mime_type"`
	SizeBytes  int64     `json:"size_bytes"`
	OCRText    string    `json:"ocr_text,omitempty"`
	Visibility string    `json:"visibility"`
	CreatedAt  time.Time `json:"created_at"`
}

type UploadedSupportIssueAttachment struct {
	ID        int64     `json:"id"`
	FileURL   string    `json:"file_url"`
	FileName  string    `json:"file_name"`
	MimeType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

type AdminSupportIssue struct {
	ID                     int64                         `json:"id"`
	PublicID               string                        `json:"public_id"`
	Title                  string                        `json:"title"`
	Description            string                        `json:"description"`
	AccountEmail           string                        `json:"account_email"`
	AccountEmailNormalized string                        `json:"account_email_normalized"`
	AccountEmailMasked     string                        `json:"account_email_masked"`
	OccurredAt             time.Time                     `json:"occurred_at"`
	ScreenshotText         string                        `json:"screenshot_text"`
	ScreenshotLanguage     string                        `json:"screenshot_language"`
	Category               string                        `json:"category"`
	Severity               string                        `json:"severity"`
	Status                 string                        `json:"status"`
	ModelName              string                        `json:"model_name,omitempty"`
	ClientName             string                        `json:"client_name,omitempty"`
	HTTPStatus             *int                          `json:"http_status,omitempty"`
	ErrorCode              string                        `json:"error_code,omitempty"`
	APIKeySuffix           string                        `json:"api_key_suffix,omitempty"`
	CreatedByUserID        *int64                        `json:"created_by_user_id,omitempty"`
	ResolvedByUserID       *int64                        `json:"resolved_by_user_id,omitempty"`
	ResolvedAt             *time.Time                    `json:"resolved_at,omitempty"`
	LockedAt               *time.Time                    `json:"locked_at,omitempty"`
	HiddenAt               *time.Time                    `json:"hidden_at,omitempty"`
	HiddenByUserID         *int64                        `json:"hidden_by_user_id,omitempty"`
	HideReason             string                        `json:"hide_reason,omitempty"`
	PinnedAt               *time.Time                    `json:"pinned_at,omitempty"`
	PinnedByUserID         *int64                        `json:"pinned_by_user_id,omitempty"`
	PinnedReason           string                        `json:"pinned_reason,omitempty"`
	SolutionCommentID      *int64                        `json:"solution_comment_id,omitempty"`
	RelatedIssueID         *int64                        `json:"related_issue_id,omitempty"`
	RelatedIssueReason     string                        `json:"related_issue_reason,omitempty"`
	LastCommentAt          *time.Time                    `json:"last_comment_at,omitempty"`
	LastViewedAt           *time.Time                    `json:"last_viewed_at,omitempty"`
	CommentCount           int                           `json:"comment_count"`
	HiddenCommentCount     int                           `json:"hidden_comment_count"`
	AttachmentCount        int                           `json:"attachment_count"`
	ViewCount              int                           `json:"view_count"`
	CreatedAt              time.Time                     `json:"created_at"`
	UpdatedAt              time.Time                     `json:"updated_at"`
	Comments               []AdminSupportIssueComment    `json:"comments,omitempty"`
	Attachments            []AdminSupportIssueAttachment `json:"attachments,omitempty"`
	Events                 []SupportIssueEvent           `json:"events,omitempty"`
	SolutionComment        *AdminSupportIssueComment     `json:"solution_comment,omitempty"`
	RelatedIssue           *SupportIssueReference        `json:"related_issue,omitempty"`
}

type AdminSupportIssueComment struct {
	ID             int64                  `json:"id"`
	IssueID        int64                  `json:"issue_id"`
	AuthorUserID   *int64                 `json:"author_user_id,omitempty"`
	AuthorRole     string                 `json:"author_role"`
	Content        string                 `json:"content"`
	RelatedIssueID *int64                 `json:"related_issue_id,omitempty"`
	HiddenAt       *time.Time             `json:"hidden_at,omitempty"`
	HiddenByUserID *int64                 `json:"hidden_by_user_id,omitempty"`
	HideReason     string                 `json:"hide_reason,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	RelatedIssue   *SupportIssueReference `json:"related_issue,omitempty"`
}

type AdminSupportIssueAttachment struct {
	ID               int64      `json:"id"`
	IssueID          int64      `json:"issue_id"`
	UploadedByUserID *int64     `json:"uploaded_by_user_id,omitempty"`
	FilePath         string     `json:"file_path,omitempty"`
	FileURL          string     `json:"file_url"`
	FileName         string     `json:"file_name"`
	MimeType         string     `json:"mime_type"`
	SizeBytes        int64      `json:"size_bytes"`
	OCRText          string     `json:"ocr_text,omitempty"`
	Visibility       string     `json:"visibility"`
	HiddenAt         *time.Time `json:"hidden_at,omitempty"`
	HiddenByUserID   *int64     `json:"hidden_by_user_id,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type SupportIssueEvent struct {
	ID          int64          `json:"id"`
	IssueID     int64          `json:"issue_id"`
	ActorUserID *int64         `json:"actor_user_id,omitempty"`
	EventType   string         `json:"event_type"`
	FromStatus  *string        `json:"from_status,omitempty"`
	ToStatus    *string        `json:"to_status,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

func (r CreateSupportIssueRequest) ToServiceInput() service.CreateSupportIssueInput {
	return service.CreateSupportIssueInput{
		Title:              r.Title,
		Description:        r.Description,
		AccountEmail:       r.AccountEmail,
		OccurredAt:         r.OccurredAt,
		ScreenshotText:     r.ScreenshotText,
		ScreenshotLanguage: r.ScreenshotLanguage,
		Category:           r.Category,
		Severity:           r.Severity,
		ModelName:          r.ModelName,
		ClientName:         r.ClientName,
		HTTPStatus:         r.HTTPStatus,
		ErrorCode:          r.ErrorCode,
		APIKeySuffix:       r.APIKeySuffix,
		AttachmentIDs:      append([]int64(nil), r.AttachmentIDs...),
	}
}

func (r SupportIssueAttachmentRequest) ToServiceAttachment() service.SupportIssueAttachment {
	return service.SupportIssueAttachment{
		FileURL:    r.FileURL,
		FileName:   r.FileName,
		MimeType:   r.MimeType,
		SizeBytes:  r.SizeBytes,
		OCRText:    r.OCRText,
		Visibility: service.SupportIssueAttachmentVisibilityPublic,
	}
}

func PublicSupportIssueFromService(issue *service.SupportIssue) *PublicSupportIssue {
	if issue == nil {
		return nil
	}
	out := &PublicSupportIssue{
		ID:                 issue.ID,
		PublicID:           issue.PublicID,
		Title:              issue.Title,
		Description:        issue.Description,
		AccountEmailMasked: issue.AccountEmailMasked,
		OccurredAt:         issue.OccurredAt,
		ScreenshotText:     issue.ScreenshotText,
		ScreenshotLanguage: issue.ScreenshotLanguage,
		Category:           issue.Category,
		Severity:           issue.Severity,
		Status:             issue.Status,
		ModelName:          issue.ModelName,
		ClientName:         issue.ClientName,
		HTTPStatus:         issue.HTTPStatus,
		ErrorCode:          issue.ErrorCode,
		ResolvedAt:         issue.ResolvedAt,
		LockedAt:           issue.LockedAt,
		PinnedAt:           issue.PinnedAt,
		SolutionCommentID:  issue.SolutionCommentID,
		RelatedIssueID:     issue.RelatedIssueID,
		LastCommentAt:      issue.LastCommentAt,
		LastViewedAt:       issue.LastViewedAt,
		CommentCount:       issue.CommentCount,
		AttachmentCount:    issue.AttachmentCount,
		ViewCount:          issue.ViewCount,
		CreatedAt:          issue.CreatedAt,
		UpdatedAt:          issue.UpdatedAt,
		Comments:           PublicSupportIssueCommentsFromService(issue.Comments),
		Attachments:        PublicSupportIssueAttachmentsFromService(issue.Attachments),
		SolutionComment:    PublicSupportIssueCommentFromService(issue.SolutionComment),
		RelatedIssue:       SupportIssueReferenceFromService(issue.RelatedIssue),
	}
	return out
}

func PublicSupportIssuesFromService(items []service.SupportIssue) []PublicSupportIssue {
	out := make([]PublicSupportIssue, 0, len(items))
	for i := range items {
		if item := PublicSupportIssueFromService(&items[i]); item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func PublicSupportIssueCommentsFromService(items []service.SupportIssueComment) []PublicSupportIssueComment {
	out := make([]PublicSupportIssueComment, 0, len(items))
	for i := range items {
		if item := PublicSupportIssueCommentFromService(&items[i]); item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func PublicSupportIssueCommentFromService(item *service.SupportIssueComment) *PublicSupportIssueComment {
	if item == nil {
		return nil
	}
	return &PublicSupportIssueComment{
		ID:             item.ID,
		IssueID:        item.IssueID,
		AuthorRole:     item.AuthorRole,
		Content:        item.Content,
		RelatedIssueID: item.RelatedIssueID,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
		RelatedIssue:   SupportIssueReferenceFromService(item.RelatedIssue),
	}
}

func PublicSupportIssueAttachmentsFromService(items []service.SupportIssueAttachment) []PublicSupportIssueAttachment {
	out := make([]PublicSupportIssueAttachment, 0, len(items))
	for i := range items {
		if item := PublicSupportIssueAttachmentFromService(&items[i]); item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func PublicSupportIssueAttachmentFromService(item *service.SupportIssueAttachment) *PublicSupportIssueAttachment {
	if item == nil {
		return nil
	}
	return &PublicSupportIssueAttachment{
		ID:         item.ID,
		IssueID:    item.IssueID,
		FileURL:    item.FileURL,
		FileName:   item.FileName,
		MimeType:   item.MimeType,
		SizeBytes:  item.SizeBytes,
		OCRText:    item.OCRText,
		Visibility: item.Visibility,
		CreatedAt:  item.CreatedAt,
	}
}

func UploadedSupportIssueAttachmentFromService(item *service.SupportIssueAttachment) *UploadedSupportIssueAttachment {
	if item == nil {
		return nil
	}
	return &UploadedSupportIssueAttachment{
		ID:        item.ID,
		FileURL:   item.FileURL,
		FileName:  item.FileName,
		MimeType:  item.MimeType,
		SizeBytes: item.SizeBytes,
		CreatedAt: item.CreatedAt,
	}
}

func AdminSupportIssueFromService(issue *service.SupportIssue) *AdminSupportIssue {
	if issue == nil {
		return nil
	}
	return &AdminSupportIssue{
		ID:                     issue.ID,
		PublicID:               issue.PublicID,
		Title:                  issue.Title,
		Description:            issue.Description,
		AccountEmail:           issue.AccountEmail,
		AccountEmailNormalized: issue.AccountEmailNormalized,
		AccountEmailMasked:     issue.AccountEmailMasked,
		OccurredAt:             issue.OccurredAt,
		ScreenshotText:         issue.ScreenshotText,
		ScreenshotLanguage:     issue.ScreenshotLanguage,
		Category:               issue.Category,
		Severity:               issue.Severity,
		Status:                 issue.Status,
		ModelName:              issue.ModelName,
		ClientName:             issue.ClientName,
		HTTPStatus:             issue.HTTPStatus,
		ErrorCode:              issue.ErrorCode,
		APIKeySuffix:           issue.APIKeySuffix,
		CreatedByUserID:        issue.CreatedByUserID,
		ResolvedByUserID:       issue.ResolvedByUserID,
		ResolvedAt:             issue.ResolvedAt,
		LockedAt:               issue.LockedAt,
		HiddenAt:               issue.HiddenAt,
		HiddenByUserID:         issue.HiddenByUserID,
		HideReason:             issue.HideReason,
		PinnedAt:               issue.PinnedAt,
		PinnedByUserID:         issue.PinnedByUserID,
		PinnedReason:           issue.PinnedReason,
		SolutionCommentID:      issue.SolutionCommentID,
		RelatedIssueID:         issue.RelatedIssueID,
		RelatedIssueReason:     issue.RelatedIssueReason,
		LastCommentAt:          issue.LastCommentAt,
		LastViewedAt:           issue.LastViewedAt,
		CommentCount:           issue.CommentCount,
		HiddenCommentCount:     issue.HiddenCommentCount,
		AttachmentCount:        issue.AttachmentCount,
		ViewCount:              issue.ViewCount,
		CreatedAt:              issue.CreatedAt,
		UpdatedAt:              issue.UpdatedAt,
		Comments:               AdminSupportIssueCommentsFromService(issue.Comments),
		Attachments:            AdminSupportIssueAttachmentsFromService(issue.Attachments),
		Events:                 SupportIssueEventsFromService(issue.Events),
		SolutionComment:        AdminSupportIssueCommentFromService(issue.SolutionComment),
		RelatedIssue:           SupportIssueReferenceFromService(issue.RelatedIssue),
	}
}

func AdminSupportIssuesFromService(items []service.SupportIssue) []AdminSupportIssue {
	out := make([]AdminSupportIssue, 0, len(items))
	for i := range items {
		if item := AdminSupportIssueFromService(&items[i]); item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func AdminSupportIssueCommentsFromService(items []service.SupportIssueComment) []AdminSupportIssueComment {
	out := make([]AdminSupportIssueComment, 0, len(items))
	for i := range items {
		if item := AdminSupportIssueCommentFromService(&items[i]); item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func AdminSupportIssueCommentFromService(item *service.SupportIssueComment) *AdminSupportIssueComment {
	if item == nil {
		return nil
	}
	return &AdminSupportIssueComment{
		ID:             item.ID,
		IssueID:        item.IssueID,
		AuthorUserID:   item.AuthorUserID,
		AuthorRole:     item.AuthorRole,
		Content:        item.Content,
		RelatedIssueID: item.RelatedIssueID,
		HiddenAt:       item.HiddenAt,
		HiddenByUserID: item.HiddenByUserID,
		HideReason:     item.HideReason,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
		RelatedIssue:   SupportIssueReferenceFromService(item.RelatedIssue),
	}
}

func SupportIssueReferenceFromService(item *service.SupportIssueReference) *SupportIssueReference {
	if item == nil {
		return nil
	}
	return &SupportIssueReference{
		ID:         item.ID,
		PublicID:   item.PublicID,
		Title:      item.Title,
		Status:     item.Status,
		ResolvedAt: item.ResolvedAt,
	}
}

func AdminSupportIssueAttachmentsFromService(items []service.SupportIssueAttachment) []AdminSupportIssueAttachment {
	out := make([]AdminSupportIssueAttachment, 0, len(items))
	for i := range items {
		out = append(out, AdminSupportIssueAttachment{
			ID:               items[i].ID,
			IssueID:          items[i].IssueID,
			UploadedByUserID: items[i].UploadedByUserID,
			FilePath:         items[i].FilePath,
			FileURL:          items[i].FileURL,
			FileName:         items[i].FileName,
			MimeType:         items[i].MimeType,
			SizeBytes:        items[i].SizeBytes,
			OCRText:          items[i].OCRText,
			Visibility:       items[i].Visibility,
			HiddenAt:         items[i].HiddenAt,
			HiddenByUserID:   items[i].HiddenByUserID,
			CreatedAt:        items[i].CreatedAt,
		})
	}
	return out
}

func SupportIssueEventsFromService(items []service.SupportIssueEvent) []SupportIssueEvent {
	out := make([]SupportIssueEvent, 0, len(items))
	for i := range items {
		out = append(out, SupportIssueEvent{
			ID:          items[i].ID,
			IssueID:     items[i].IssueID,
			ActorUserID: items[i].ActorUserID,
			EventType:   items[i].EventType,
			FromStatus:  items[i].FromStatus,
			ToStatus:    items[i].ToStatus,
			Metadata:    items[i].Metadata,
			CreatedAt:   items[i].CreatedAt,
		})
	}
	return out
}
