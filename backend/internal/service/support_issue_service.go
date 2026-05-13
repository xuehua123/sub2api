package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type SupportIssueService struct {
	repo SupportIssueRepository
}

var supportIssueAttachmentStorageRoot = SupportIssueAttachmentStorageDir

func NewSupportIssueService(repo SupportIssueRepository) *SupportIssueService {
	return &SupportIssueService{repo: repo}
}

func (s *SupportIssueService) Create(ctx context.Context, actor SupportIssueActor, input CreateSupportIssueInput) (*SupportIssue, error) {
	if err := validateSupportIssueActor(actor); err != nil {
		return nil, err
	}

	normalized, err := domain.ValidateSupportIssueBaseFields(domain.SupportIssueBaseFields{
		Title:              input.Title,
		Description:        input.Description,
		AccountEmail:       input.AccountEmail,
		OccurredAt:         input.OccurredAt,
		ScreenshotText:     input.ScreenshotText,
		Category:           input.Category,
		Severity:           input.Severity,
		Status:             SupportIssueStatusOpen,
		ScreenshotLanguage: input.ScreenshotLanguage,
		HTTPStatus:         input.HTTPStatus,
		APIKeySuffix:       input.APIKeySuffix,
	}, time.Now())
	if err != nil {
		return nil, err
	}

	issue := &SupportIssue{
		Title:                  normalized.Title,
		Description:            normalized.Description,
		AccountEmail:           normalized.AccountEmail,
		AccountEmailNormalized: normalized.AccountEmailNormalized,
		AccountEmailMasked:     normalized.AccountEmailMasked,
		OccurredAt:             normalized.OccurredAt,
		ScreenshotText:         normalized.ScreenshotText,
		ScreenshotLanguage:     normalized.ScreenshotLanguage,
		Category:               normalized.Category,
		Severity:               normalized.Severity,
		Status:                 SupportIssueStatusOpen,
		ModelName:              strings.TrimSpace(input.ModelName),
		ClientName:             strings.TrimSpace(input.ClientName),
		HTTPStatus:             normalized.HTTPStatus,
		ErrorCode:              strings.TrimSpace(input.ErrorCode),
		APIKeySuffix:           normalized.APIKeySuffix,
		CreatedByUserID:        supportIssuePtrInt64(actor.UserID),
	}
	issue.SearchText = buildSupportIssueSearchText(issue)

	attachments, err := s.supportIssueCreateAttachments(ctx, actor, input.AttachmentIDs)
	if err != nil {
		return nil, err
	}
	event := supportIssueEvent(actor, SupportIssueEventCreated, map[string]any{
		"actor_role": supportIssueActorRole(actor),
	})
	if err := s.repo.CreateIssue(ctx, issue, attachments, event); err != nil {
		return nil, err
	}
	return issue, nil
}

func (s *SupportIssueService) UploadAttachment(ctx context.Context, actor SupportIssueActor, input UploadSupportIssueAttachmentInput) (*SupportIssueAttachment, error) {
	if err := validateSupportIssueActor(actor); err != nil {
		return nil, err
	}

	mimeType, err := validateSupportIssueAttachmentContent(input.ContentType, input.Content)
	if err != nil {
		return nil, err
	}
	fileName := sanitizeSupportIssueAttachmentFileName(input.FileName)
	filePath, err := writeSupportIssueAttachmentFile(mimeType, input.Content)
	if err != nil {
		return nil, err
	}

	attachment := &SupportIssueAttachment{
		UploadedByUserID: supportIssuePtrInt64(actor.UserID),
		FilePath:         filePath,
		FileName:         fileName,
		MimeType:         mimeType,
		SizeBytes:        int64(len(input.Content)),
		Visibility:       SupportIssueAttachmentVisibilityPublic,
	}
	if err := s.repo.CreateUnboundAttachment(ctx, attachment); err != nil {
		_ = os.Remove(filePath)
		return nil, err
	}
	out := sanitizeSupportIssueAttachmentForPublic(*attachment)
	return &out, nil
}

func (s *SupportIssueService) ListPublic(
	ctx context.Context,
	params pagination.PaginationParams,
	filters ListSupportIssueFilters,
) ([]SupportIssue, *pagination.PaginationResult, error) {
	filters.IncludeHidden = false
	hidden := false
	filters.Hidden = &hidden
	items, page, err := s.repo.ListIssues(ctx, params, filters)
	if err != nil {
		return nil, nil, err
	}
	return sanitizeSupportIssuesForPublic(items), page, nil
}

func (s *SupportIssueService) SearchPublic(
	ctx context.Context,
	params pagination.PaginationParams,
	rawQuery string,
	filters ListSupportIssueFilters,
) ([]SupportIssue, *pagination.PaginationResult, error) {
	filters.IncludeHidden = false
	hidden := false
	filters.Hidden = &hidden
	parsed, err := ParseSupportIssueSearch(rawQuery)
	if err != nil {
		return nil, nil, err
	}
	if parsed.APIKeySuffix != "" {
		return nil, nil, ErrSupportIssueSearchInvalid
	}
	items, page, err := s.repo.SearchIssues(ctx, params, SearchSupportIssueQuery{
		Parsed:        parsed,
		Filters:       filters,
		IncludeHidden: false,
	})
	if err != nil {
		return nil, nil, err
	}
	return sanitizeSupportIssuesForPublic(items), page, nil
}

func (s *SupportIssueService) GetPublic(ctx context.Context, issueID int64, viewer SupportIssueViewer) (*SupportIssue, error) {
	issue, err := s.repo.GetIssue(ctx, issueID, false)
	if err != nil {
		return nil, err
	}
	if issue.HiddenAt != nil {
		return nil, ErrSupportIssueNotFound
	}
	if err := s.repo.RecordView(ctx, issueID, viewer, SupportIssueViewThrottleWindow); err != nil {
		return nil, err
	}
	return sanitizeSupportIssueForPublic(issue), nil
}

func (s *SupportIssueService) OpenAttachmentForPublic(ctx context.Context, attachmentID int64) (*SupportIssueAttachment, error) {
	if attachmentID <= 0 {
		return nil, ErrSupportIssueAttachmentNotFound
	}
	attachment, err := s.repo.OpenAttachmentForPublic(ctx, attachmentID)
	if err != nil {
		return nil, err
	}
	if attachment == nil ||
		attachment.IssueID <= 0 ||
		attachment.Visibility != SupportIssueAttachmentVisibilityPublic ||
		attachment.HiddenAt != nil ||
		attachment.FilePath == "" {
		return nil, ErrSupportIssueAttachmentNotFound
	}
	filePath, err := supportIssueSafeAttachmentPath(attachment.FilePath)
	if err != nil {
		return nil, ErrSupportIssueAttachmentNotFound
	}
	attachment.FilePath = filePath
	return attachment, nil
}

func (s *SupportIssueService) AddComment(ctx context.Context, actor SupportIssueActor, issueID int64, content string) (*SupportIssueComment, error) {
	if err := validateSupportIssueActor(actor); err != nil {
		return nil, err
	}

	issue, err := s.repo.GetIssue(ctx, issueID, false)
	if err != nil {
		return nil, err
	}
	if issue.HiddenAt != nil {
		return nil, ErrSupportIssueNotFound
	}
	if issue.LockedAt != nil || domain.IsSupportIssueLockedStatus(issue.Status) {
		return nil, ErrSupportIssueLocked
	}

	trimmed, err := validateSupportIssueCommentContent(content)
	if err != nil {
		return nil, err
	}

	comment := &SupportIssueComment{
		IssueID:      issueID,
		AuthorUserID: supportIssuePtrInt64(actor.UserID),
		AuthorRole:   supportIssueActorRole(actor),
		Content:      trimmed,
	}
	event := supportIssueEvent(actor, SupportIssueEventCommented, nil)
	if err := s.repo.AddComment(ctx, comment, event); err != nil {
		return nil, err
	}
	return comment, nil
}

func (s *SupportIssueService) Resolve(ctx context.Context, actor SupportIssueActor, issueID int64) (*SupportIssue, error) {
	if err := validateSupportIssueActor(actor); err != nil {
		return nil, err
	}

	issue, err := s.repo.GetIssue(ctx, issueID, false)
	if err != nil {
		return nil, err
	}
	if issue.HiddenAt != nil {
		return nil, ErrSupportIssueNotFound
	}
	if !supportIssueActorIsAdmin(actor) && !supportIssueActorOwnsIssue(actor, issue) {
		return nil, ErrSupportIssuePermissionDenied
	}

	transitionActor := domain.SupportIssueTransitionActorReporter
	if supportIssueActorIsAdmin(actor) {
		transitionActor = domain.SupportIssueTransitionActorAdmin
	}
	if err := domain.ValidateSupportIssueStatusTransition(issue.Status, SupportIssueStatusResolved, transitionActor); err != nil {
		return nil, err
	}

	return s.repo.UpdateStatus(ctx, issueID, SupportIssueStatusResolved, actor.UserID, supportIssueActorIsAdmin(actor), supportIssueStatusEvent(actor, issue.Status, SupportIssueStatusResolved, ""))
}

func (s *SupportIssueService) AdminList(
	ctx context.Context,
	params pagination.PaginationParams,
	filters ListSupportIssueFilters,
) ([]SupportIssue, *pagination.PaginationResult, error) {
	filters.IncludeHidden = true
	return s.repo.ListIssues(ctx, params, filters)
}

func (s *SupportIssueService) AdminSearch(
	ctx context.Context,
	params pagination.PaginationParams,
	rawQuery string,
	filters ListSupportIssueFilters,
) ([]SupportIssue, *pagination.PaginationResult, error) {
	parsed, err := ParseSupportIssueSearch(rawQuery)
	if err != nil {
		return nil, nil, err
	}
	filters.IncludeHidden = true
	return s.repo.SearchIssues(ctx, params, SearchSupportIssueQuery{
		Parsed:        parsed,
		Filters:       filters,
		IncludeHidden: true,
	})
}

func (s *SupportIssueService) AdminGet(ctx context.Context, issueID int64) (*SupportIssue, error) {
	return s.repo.GetIssue(ctx, issueID, true)
}

func (s *SupportIssueService) AdminUpdateStatus(
	ctx context.Context,
	actor SupportIssueActor,
	issueID int64,
	nextStatus string,
	reason string,
) (*SupportIssue, error) {
	if err := validateSupportIssueAdmin(actor); err != nil {
		return nil, err
	}

	issue, err := s.repo.GetIssue(ctx, issueID, true)
	if err != nil {
		return nil, err
	}
	status, err := domain.ValidateSupportIssueStatus(nextStatus)
	if err != nil {
		return nil, err
	}
	if err := domain.ValidateSupportIssueStatusTransition(issue.Status, status, domain.SupportIssueTransitionActorAdmin); err != nil {
		return nil, err
	}

	return s.repo.UpdateStatus(ctx, issueID, status, actor.UserID, true, supportIssueStatusEvent(actor, issue.Status, status, reason))
}

func (s *SupportIssueService) AdminReopen(ctx context.Context, actor SupportIssueActor, issueID int64, reason string) (*SupportIssue, error) {
	if err := validateSupportIssueAdmin(actor); err != nil {
		return nil, err
	}

	issue, err := s.repo.GetIssue(ctx, issueID, true)
	if err != nil {
		return nil, err
	}
	if err := domain.ValidateSupportIssueStatusTransition(issue.Status, SupportIssueStatusOpen, domain.SupportIssueTransitionActorAdmin); err != nil {
		return nil, err
	}

	return s.repo.UpdateStatus(ctx, issueID, SupportIssueStatusOpen, actor.UserID, true, supportIssueStatusEventWithType(actor, SupportIssueEventReopened, issue.Status, SupportIssueStatusOpen, reason))
}

func (s *SupportIssueService) AdminHideIssue(ctx context.Context, actor SupportIssueActor, issueID int64, reason string) (*SupportIssue, error) {
	if err := validateSupportIssueAdmin(actor); err != nil {
		return nil, err
	}
	reason = supportIssueHideReason(reason)
	return s.repo.HideIssue(ctx, HideSupportIssueInput{
		IssueID:        issueID,
		HiddenByUserID: actor.UserID,
		HideReason:     reason,
	}, supportIssueEvent(actor, SupportIssueEventIssueHidden, map[string]any{
		"reason": reason,
	}))
}

func (s *SupportIssueService) AdminRestoreIssue(ctx context.Context, actor SupportIssueActor, issueID int64, reason string) (*SupportIssue, error) {
	if err := validateSupportIssueAdmin(actor); err != nil {
		return nil, err
	}
	return s.repo.RestoreIssue(ctx, issueID, actor.UserID, supportIssueEvent(actor, SupportIssueEventIssueRestored, map[string]any{
		"reason": strings.TrimSpace(reason),
	}))
}

func (s *SupportIssueService) AdminHideComment(ctx context.Context, actor SupportIssueActor, issueID int64, commentID int64, reason string) error {
	if err := validateSupportIssueAdmin(actor); err != nil {
		return err
	}
	reason = supportIssueHideReason(reason)
	return s.repo.HideComment(ctx, HideSupportIssueCommentInput{
		IssueID:        issueID,
		CommentID:      commentID,
		HiddenByUserID: actor.UserID,
		HideReason:     reason,
	}, supportIssueEvent(actor, SupportIssueEventCommentHidden, map[string]any{
		"reason": reason,
	}))
}

func (s *SupportIssueService) AdminHideAttachment(ctx context.Context, actor SupportIssueActor, issueID int64, attachmentID int64, reason string) error {
	if err := validateSupportIssueAdmin(actor); err != nil {
		return err
	}
	reason = supportIssueHideReason(reason)
	return s.repo.HideAttachment(ctx, HideSupportIssueAttachmentInput{
		IssueID:        issueID,
		AttachmentID:   attachmentID,
		HiddenByUserID: actor.UserID,
	}, supportIssueEvent(actor, SupportIssueEventAttachmentHidden, map[string]any{
		"reason": reason,
	}))
}

func (s *SupportIssueService) AdminListEvents(ctx context.Context, issueID int64) ([]SupportIssueEvent, error) {
	return s.repo.ListEvents(ctx, issueID)
}

func (s *SupportIssueService) SuggestSimilar(ctx context.Context, actor SupportIssueActor, input CreateSupportIssueInput) ([]SupportIssue, error) {
	if err := validateSupportIssueActor(actor); err != nil {
		return nil, err
	}

	parsed := ParsedSupportIssueSearch{
		ErrorCode: strings.TrimSpace(input.ErrorCode),
		Terms:     supportIssueSuggestionTerms(input.Title, input.ScreenshotText),
	}
	if input.HTTPStatus != nil {
		code := *input.HTTPStatus
		parsed.HTTPStatus = &code
	}

	items, _, err := s.repo.SearchIssues(ctx, pagination.PaginationParams{Page: 1, PageSize: 5}, SearchSupportIssueQuery{
		Parsed:        parsed,
		IncludeHidden: false,
	})
	if err != nil {
		return nil, err
	}
	items = sanitizeSupportIssuesForPublic(items)
	if len(items) > 5 {
		return items[:5], nil
	}
	return items, nil
}

func validateSupportIssueActor(actor SupportIssueActor) error {
	if actor.UserID <= 0 {
		return ErrSupportIssuePermissionDenied
	}
	return nil
}

func validateSupportIssueAdmin(actor SupportIssueActor) error {
	if err := validateSupportIssueActor(actor); err != nil {
		return err
	}
	if !supportIssueActorIsAdmin(actor) {
		return ErrSupportIssuePermissionDenied
	}
	return nil
}

func validateSupportIssueCommentContent(content string) (string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" || len([]rune(trimmed)) > SupportIssueMaxCommentContentRunes {
		return "", ErrSupportIssueInvalidInput
	}
	return trimmed, nil
}

func buildSupportIssueSearchText(issue *SupportIssue) string {
	fields := []string{
		issue.Title,
		issue.Description,
		issue.AccountEmailMasked,
		issue.AccountEmailNormalized,
		issue.ScreenshotText,
		issue.Category,
		issue.Severity,
		issue.ModelName,
		issue.ClientName,
		issue.ErrorCode,
	}
	if issue.HTTPStatus != nil {
		fields = append(fields, strconv.Itoa(*issue.HTTPStatus))
	}

	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			parts = append(parts, field)
		}
	}
	return strings.Join(parts, " ")
}

func supportIssueActorIsAdmin(actor SupportIssueActor) bool {
	return actor.IsAdmin || actor.Role == RoleAdmin
}

func supportIssueActorRole(actor SupportIssueActor) string {
	if supportIssueActorIsAdmin(actor) {
		return RoleAdmin
	}
	return RoleUser
}

func supportIssueActorOwnsIssue(actor SupportIssueActor, issue *SupportIssue) bool {
	return issue != nil && issue.CreatedByUserID != nil && *issue.CreatedByUserID == actor.UserID
}

func supportIssueEvent(actor SupportIssueActor, eventType string, metadata map[string]any) SupportIssueEvent {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return SupportIssueEvent{
		ActorUserID: supportIssuePtrInt64(actor.UserID),
		EventType:   eventType,
		Metadata:    metadata,
	}
}

func supportIssueStatusEvent(actor SupportIssueActor, fromStatus string, toStatus string, reason string) SupportIssueEvent {
	return supportIssueStatusEventWithType(actor, SupportIssueEventStatusChanged, fromStatus, toStatus, reason)
}

func supportIssueStatusEventWithType(actor SupportIssueActor, eventType string, fromStatus string, toStatus string, reason string) SupportIssueEvent {
	metadata := map[string]any{}
	if strings.TrimSpace(reason) != "" {
		metadata["reason"] = strings.TrimSpace(reason)
	}
	event := supportIssueEvent(actor, eventType, metadata)
	event.FromStatus = supportIssuePtrString(fromStatus)
	event.ToStatus = supportIssuePtrString(toStatus)
	return event
}

func supportIssueHideReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return SupportIssueDefaultHideReason
	}
	return reason
}

func supportIssueSuggestionTerms(values ...string) []string {
	const maxTerms = 8

	seen := make(map[string]struct{})
	terms := make([]string, 0, maxTerms)
	for _, value := range values {
		for _, field := range strings.Fields(strings.ToLower(value)) {
			field = strings.Trim(field, " \t\r\n.,;:!?()[]{}\"'")
			if len([]rune(field)) < 2 {
				continue
			}
			if _, ok := seen[field]; ok {
				continue
			}
			seen[field] = struct{}{}
			terms = append(terms, field)
			if len(terms) >= maxTerms {
				return terms
			}
		}
	}
	return terms
}

func sanitizeSupportIssuesForPublic(items []SupportIssue) []SupportIssue {
	out := make([]SupportIssue, 0, len(items))
	for i := range items {
		item := items[i]
		if sanitized := sanitizeSupportIssueForPublic(&item); sanitized != nil {
			out = append(out, *sanitized)
		}
	}
	return out
}

func sanitizeSupportIssueForPublic(issue *SupportIssue) *SupportIssue {
	if issue == nil {
		return nil
	}
	out := *issue
	out.AccountEmail = ""
	out.AccountEmailNormalized = ""
	out.APIKeySuffix = ""
	out.CreatedByUserID = nil
	out.ResolvedByUserID = nil
	out.HiddenCommentCount = 0
	out.HiddenAt = nil
	out.HiddenByUserID = nil
	out.HideReason = ""
	out.Comments = visibleSupportIssueComments(issue.Comments)
	out.Attachments = visibleSupportIssueAttachments(issue.Attachments)
	out.Events = nil
	return &out
}

func visibleSupportIssueComments(comments []SupportIssueComment) []SupportIssueComment {
	out := make([]SupportIssueComment, 0, len(comments))
	for _, comment := range comments {
		if comment.HiddenAt == nil {
			out = append(out, sanitizeSupportIssueCommentForPublic(comment))
		}
	}
	return out
}

func visibleSupportIssueAttachments(attachments []SupportIssueAttachment) []SupportIssueAttachment {
	out := make([]SupportIssueAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Visibility == "" || attachment.Visibility == SupportIssueAttachmentVisibilityPublic {
			out = append(out, sanitizeSupportIssueAttachmentForPublic(attachment))
		}
	}
	return out
}

func sanitizeSupportIssueCommentForPublic(comment SupportIssueComment) SupportIssueComment {
	return SupportIssueComment{
		ID:         comment.ID,
		IssueID:    comment.IssueID,
		AuthorRole: comment.AuthorRole,
		Content:    comment.Content,
		CreatedAt:  comment.CreatedAt,
		UpdatedAt:  comment.UpdatedAt,
	}
}

func sanitizeSupportIssueAttachmentForPublic(attachment SupportIssueAttachment) SupportIssueAttachment {
	out := SupportIssueAttachment{
		ID:         attachment.ID,
		IssueID:    attachment.IssueID,
		FileURL:    attachment.FileURL,
		FileName:   attachment.FileName,
		MimeType:   attachment.MimeType,
		SizeBytes:  attachment.SizeBytes,
		OCRText:    attachment.OCRText,
		Visibility: attachment.Visibility,
		CreatedAt:  attachment.CreatedAt,
	}
	return out
}

func (s *SupportIssueService) supportIssueCreateAttachments(
	ctx context.Context,
	actor SupportIssueActor,
	ids []int64,
) ([]SupportIssueAttachment, error) {
	normalizedIDs, err := normalizeSupportIssueAttachmentIDs(ids)
	if err != nil {
		return nil, err
	}
	if len(normalizedIDs) == 0 {
		return nil, nil
	}

	attachments, err := s.repo.ListUnboundAttachmentsForUser(ctx, actor.UserID, normalizedIDs)
	if err != nil {
		return nil, err
	}
	if len(attachments) != len(normalizedIDs) {
		return nil, ErrSupportIssueAttachmentNotFound
	}

	byID := make(map[int64]SupportIssueAttachment, len(attachments))
	for _, attachment := range attachments {
		if attachment.ID <= 0 ||
			attachment.IssueID != 0 ||
			attachment.UploadedByUserID == nil ||
			*attachment.UploadedByUserID != actor.UserID ||
			attachment.Visibility != SupportIssueAttachmentVisibilityPublic ||
			attachment.HiddenAt != nil {
			return nil, ErrSupportIssueAttachmentNotFound
		}
		byID[attachment.ID] = attachment
	}

	ordered := make([]SupportIssueAttachment, 0, len(normalizedIDs))
	for _, id := range normalizedIDs {
		attachment, ok := byID[id]
		if !ok {
			return nil, ErrSupportIssueAttachmentNotFound
		}
		ordered = append(ordered, attachment)
	}
	return ordered, nil
}

func normalizeSupportIssueAttachmentIDs(ids []int64) ([]int64, error) {
	if len(ids) > SupportIssueMaxAttachmentsPerIssue {
		return nil, ErrSupportIssueInvalidInput
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrSupportIssueAttachmentNotFound
		}
		if _, ok := seen[id]; ok {
			return nil, ErrSupportIssueInvalidInput
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

func validateSupportIssueAttachmentContent(declaredType string, content []byte) (string, error) {
	if len(content) == 0 {
		return "", ErrSupportIssueAttachmentInvalidType
	}
	if len(content) > SupportIssueMaxAttachmentBytes {
		return "", ErrSupportIssueAttachmentTooLarge
	}

	declaredType = normalizeSupportIssueAttachmentContentType(declaredType)
	if declaredType != "" && !supportIssueAttachmentMimeAllowed(declaredType) {
		return "", ErrSupportIssueAttachmentInvalidType
	}

	detectedType := detectSupportIssueAttachmentContentType(content)
	if !supportIssueAttachmentMimeAllowed(detectedType) {
		return "", ErrSupportIssueAttachmentInvalidType
	}
	if declaredType != "" && declaredType != detectedType {
		return "", ErrSupportIssueAttachmentInvalidType
	}
	return detectedType, nil
}

func normalizeSupportIssueAttachmentContentType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if contentType == "" {
		return ""
	}
	if mediaType, _, ok := strings.Cut(contentType, ";"); ok {
		return strings.TrimSpace(mediaType)
	}
	return contentType
}

func detectSupportIssueAttachmentContentType(content []byte) string {
	if len(content) >= 12 &&
		string(content[0:4]) == "RIFF" &&
		string(content[8:12]) == "WEBP" {
		return "image/webp"
	}
	return http.DetectContentType(content)
}

func supportIssueAttachmentMimeAllowed(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return true
	default:
		return false
	}
}

func sanitizeSupportIssueAttachmentFileName(fileName string) string {
	fileName = strings.ReplaceAll(fileName, "\\", "/")
	fileName = filepath.Base(fileName)
	fileName = strings.TrimSpace(fileName)

	var b strings.Builder
	for _, r := range fileName {
		switch {
		case unicode.IsControl(r):
			continue
		case strings.ContainsRune(`/\<>:"|?*`, r):
			_, _ = b.WriteRune('_')
		default:
			_, _ = b.WriteRune(r)
		}
	}

	cleaned := strings.Trim(b.String(), " .")
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		cleaned = "attachment"
	}
	runes := []rune(cleaned)
	if len(runes) > 255 {
		cleaned = string(runes[:255])
	}
	return cleaned
}

func writeSupportIssueAttachmentFile(mimeType string, content []byte) (string, error) {
	storageRoot, err := filepath.Abs(supportIssueAttachmentStorageRoot)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(storageRoot, 0o750); err != nil {
		return "", err
	}

	name, err := randomSupportIssueAttachmentFileName(mimeType)
	if err != nil {
		return "", err
	}
	filePath := filepath.Join(storageRoot, name)
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		return "", err
	}
	return filePath, nil
}

func randomSupportIssueAttachmentFileName(mimeType string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	ext := ".bin"
	switch mimeType {
	case "image/png":
		ext = ".png"
	case "image/jpeg":
		ext = ".jpg"
	case "image/webp":
		ext = ".webp"
	case "image/gif":
		ext = ".gif"
	}
	return hex.EncodeToString(buf) + ext, nil
}

func supportIssueSafeAttachmentPath(filePath string) (string, error) {
	storageRoot, err := filepath.Abs(supportIssueAttachmentStorageRoot)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(storageRoot, absPath)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", ErrSupportIssueAttachmentNotFound
	}
	return absPath, nil
}

func supportIssuePtrInt64(v int64) *int64 {
	return &v
}

func supportIssuePtrString(v string) *string {
	return &v
}
