package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/supportissue"
	"github.com/Wei-Shaw/sub2api/ent/supportissueattachment"
	"github.com/Wei-Shaw/sub2api/ent/supportissuecomment"
	"github.com/Wei-Shaw/sub2api/ent/supportissueevent"
	"github.com/Wei-Shaw/sub2api/ent/supportissueview"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	entsql "entgo.io/ent/dialect/sql"
)

type supportIssueRepository struct {
	client *dbent.Client
}

func NewSupportIssueRepository(client *dbent.Client) service.SupportIssueRepository {
	return &supportIssueRepository{client: client}
}

func (r *supportIssueRepository) CreateIssue(
	ctx context.Context,
	issue *service.SupportIssue,
	attachments []service.SupportIssueAttachment,
	event service.SupportIssueEvent,
) error {
	if issue == nil {
		return service.ErrSupportIssueInvalidInput
	}

	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		create := buildSupportIssueCreate(txClient, issue, attachments)
		created, err := create.Save(txCtx)
		if err != nil {
			return err
		}

		publicID := fmt.Sprintf("ISS-%06d", created.ID)
		created, err = txClient.SupportIssue.UpdateOneID(created.ID).
			SetPublicID(publicID).
			Save(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
		}
		applySupportIssueEntityToService(issue, created)

		for i := range attachments {
			attachments[i].IssueID = created.ID
			var (
				att *dbent.SupportIssueAttachment
				err error
			)
			if attachments[i].ID > 0 {
				att, err = bindSupportIssueAttachment(txCtx, txClient, attachments[i], created.ID)
			} else {
				att, err = createSupportIssueAttachment(txCtx, txClient, attachments[i])
			}
			if err != nil {
				return err
			}
			issue.Attachments = append(issue.Attachments, *supportIssueAttachmentEntityToService(att))
		}

		event.IssueID = created.ID
		if event.EventType == "" {
			event.EventType = service.SupportIssueEventCreated
		}
		createdEvent, err := createSupportIssueEvent(txCtx, txClient, event)
		if err != nil {
			return err
		}
		issue.Events = append(issue.Events, *supportIssueEventEntityToService(createdEvent))
		return nil
	})
}

func (r *supportIssueRepository) CreateUnboundAttachment(ctx context.Context, attachment *service.SupportIssueAttachment) error {
	if attachment == nil {
		return service.ErrSupportIssueInvalidInput
	}

	client := clientFromContext(ctx, r.client)
	attachment.IssueID = 0
	if strings.TrimSpace(attachment.Visibility) == "" {
		attachment.Visibility = service.SupportIssueAttachmentVisibilityPublic
	}
	attachment.FileURL = "pending"

	created, err := createSupportIssueAttachment(ctx, client, *attachment)
	if err != nil {
		return err
	}
	fileURL := supportIssueAttachmentFileURL(created.ID)
	created, err = client.SupportIssueAttachment.UpdateOneID(created.ID).
		SetFileURL(fileURL).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrSupportIssueAttachmentNotFound, nil)
	}
	applySupportIssueAttachmentEntityToService(attachment, created)
	return nil
}

func (r *supportIssueRepository) ListUnboundAttachmentsForUser(
	ctx context.Context,
	userID int64,
	ids []int64,
) ([]service.SupportIssueAttachment, error) {
	if userID <= 0 || len(ids) == 0 {
		return nil, nil
	}

	client := clientFromContext(ctx, r.client)
	rows, err := client.SupportIssueAttachment.Query().
		Where(
			supportissueattachment.IDIn(ids...),
			supportissueattachment.UploadedByUserIDEQ(userID),
			supportissueattachment.IssueIDIsNil(),
			supportissueattachment.VisibilityEQ(service.SupportIssueAttachmentVisibilityPublic),
			supportissueattachment.HiddenAtIsNil(),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return supportIssueAttachmentEntitiesToService(rows), nil
}

func (r *supportIssueRepository) OpenAttachmentForPublic(ctx context.Context, attachmentID int64) (*service.SupportIssueAttachment, error) {
	client := clientFromContext(ctx, r.client)
	row, err := client.SupportIssueAttachment.Query().
		Where(
			supportissueattachment.IDEQ(attachmentID),
			supportissueattachment.IssueIDNotNil(),
			supportissueattachment.VisibilityEQ(service.SupportIssueAttachmentVisibilityPublic),
			supportissueattachment.HiddenAtIsNil(),
			supportissueattachment.HasIssueWith(supportissue.HiddenAtIsNil()),
		).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSupportIssueAttachmentNotFound, nil)
	}
	return supportIssueAttachmentEntityToService(row), nil
}

func (r *supportIssueRepository) GetIssue(ctx context.Context, id int64, includeHidden bool) (*service.SupportIssue, error) {
	client := clientFromContext(ctx, r.client)
	row, err := supportIssueQueryWithRelations(
		client.SupportIssue.Query().Where(supportissue.IDEQ(id)),
		includeHidden,
	).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
	}
	issue := supportIssueEntityToService(row)
	if err := r.populateSupportIssueReferences(ctx, client, issue, includeHidden); err != nil {
		return nil, err
	}
	return issue, nil
}

func (r *supportIssueRepository) RecordView(ctx context.Context, issueID int64, viewer service.SupportIssueViewer, throttleWindow time.Duration) error {
	if issueID <= 0 {
		return service.ErrSupportIssueNotFound
	}
	if throttleWindow <= 0 {
		throttleWindow = service.SupportIssueViewThrottleWindow
	}
	now := time.Now()
	viewerHash := supportIssueViewerHash(viewer)
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		exists, err := txClient.SupportIssue.Query().
			Where(supportissue.IDEQ(issueID), supportissue.HiddenAtIsNil()).
			Exist(txCtx)
		if err != nil {
			return err
		}
		if !exists {
			return service.ErrSupportIssueNotFound
		}

		recent, err := txClient.SupportIssueView.Query().
			Where(
				supportissueview.IssueIDEQ(issueID),
				supportissueview.ViewerHashEQ(viewerHash),
				supportissueview.ViewedAtGTE(now.Add(-throttleWindow)),
			).
			Exist(txCtx)
		if err != nil {
			return err
		}
		if recent {
			return nil
		}

		create := txClient.SupportIssueView.Create().
			SetIssueID(issueID).
			SetViewerHash(viewerHash).
			SetViewedAt(now)
		if viewer.UserID > 0 {
			create.SetViewerUserID(viewer.UserID)
		}
		if _, err := create.Save(txCtx); err != nil {
			return err
		}
		_, err = txClient.SupportIssue.UpdateOneID(issueID).
			AddViewCount(1).
			SetLastViewedAt(now).
			Save(txCtx)
		return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
	})
}

func (r *supportIssueRepository) GetUserNotificationSummary(ctx context.Context, userID int64) (service.SupportIssueNotificationSummary, error) {
	if userID <= 0 {
		return service.SupportIssueNotificationSummary{}, service.ErrSupportIssuePermissionDenied
	}

	client := clientFromContext(ctx, r.client)
	rows, err := client.SupportIssue.Query().
		Where(
			supportissue.CreatedByUserIDEQ(userID),
			supportissue.HiddenAtIsNil(),
		).
		All(ctx)
	if err != nil {
		return service.SupportIssueNotificationSummary{}, err
	}

	items := supportIssueEntitiesToService(rows)
	if err := populateSupportIssueViewerState(ctx, client, items, userID); err != nil {
		return service.SupportIssueNotificationSummary{}, err
	}

	var summary service.SupportIssueNotificationSummary
	for i := range items {
		item := &items[i]
		if item.HasUnreadActivity || item.ViewerAttentionReason == "needs_info" {
			summary.UnreadCount++
		}
		if item.Status == service.SupportIssueStatusNeedsInfo {
			summary.NeedsInfoCount++
		}
		if item.HasUnreadActivity && item.Status == service.SupportIssueStatusResolved {
			summary.ResolvedUnreadCount++
		}
		if item.ViewerLastActivityAt != nil && (summary.LatestActivityAt == nil || item.ViewerLastActivityAt.After(*summary.LatestActivityAt)) {
			latest := *item.ViewerLastActivityAt
			summary.LatestActivityAt = &latest
		}
	}
	return summary, nil
}

func (r *supportIssueRepository) ListIssues(
	ctx context.Context,
	params pagination.PaginationParams,
	filters service.ListSupportIssueFilters,
) ([]service.SupportIssue, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := applySupportIssueListFilters(client.SupportIssue.Query(), filters)

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	itemsQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range supportIssueListOrders(params) {
		itemsQuery = itemsQuery.Order(order)
	}

	rows, err := itemsQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}

	items := supportIssueEntitiesToService(rows)
	if filters.CreatedByUserID != nil {
		if err := populateSupportIssueViewerState(ctx, client, items, *filters.CreatedByUserID); err != nil {
			return nil, nil, err
		}
	}
	return items, paginationResultFromTotal(int64(total), params), nil
}

func (r *supportIssueRepository) SearchIssues(
	ctx context.Context,
	params pagination.PaginationParams,
	query service.SearchSupportIssueQuery,
) ([]service.SupportIssue, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := applySupportIssueListFilters(client.SupportIssue.Query(), query.Filters)
	q = applySupportIssueSearchQuery(q, query.Parsed)

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	itemsQuery := supportIssueQueryWithRelations(q, query.IncludeHidden).
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range supportIssueListOrders(params) {
		itemsQuery = itemsQuery.Order(order)
	}

	rows, err := itemsQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}

	items := supportIssueEntitiesToService(rows)
	if query.Filters.CreatedByUserID != nil {
		if err := populateSupportIssueViewerState(ctx, client, items, *query.Filters.CreatedByUserID); err != nil {
			return nil, nil, err
		}
	}
	return items, paginationResultFromTotal(int64(total), params), nil
}

func (r *supportIssueRepository) AddComment(
	ctx context.Context,
	comment *service.SupportIssueComment,
	event service.SupportIssueEvent,
) error {
	if comment == nil {
		return service.ErrSupportIssueInvalidInput
	}

	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		issue, err := txClient.SupportIssue.Query().
			Where(supportissue.IDEQ(comment.IssueID)).
			Only(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
		}

		created, err := createSupportIssueComment(txCtx, txClient, *comment)
		if err != nil {
			return err
		}
		applySupportIssueCommentEntityToService(comment, created)

		if comment.HiddenAt == nil {
			updated, err := txClient.SupportIssue.UpdateOneID(issue.ID).
				SetCommentCount(issue.CommentCount + 1).
				SetLastCommentAt(created.CreatedAt).
				Save(txCtx)
			if err != nil {
				return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
			}
			_ = updated
		}

		event.IssueID = issue.ID
		if event.EventType == "" {
			event.EventType = service.SupportIssueEventCommented
		}
		_, err = createSupportIssueEvent(txCtx, txClient, event)
		return err
	})
}

func (r *supportIssueRepository) UpdateStatus(
	ctx context.Context,
	issueID int64,
	nextStatus string,
	actorUserID int64,
	actorIsAdmin bool,
	event service.SupportIssueEvent,
) (*service.SupportIssue, error) {
	var out *service.SupportIssue
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		current, err := txClient.SupportIssue.Query().
			Where(supportissue.IDEQ(issueID)).
			Only(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
		}

		actor := domain.SupportIssueTransitionActorReporter
		if actorIsAdmin {
			actor = domain.SupportIssueTransitionActorAdmin
		}
		if err := domain.ValidateSupportIssueStatusTransition(current.Status, nextStatus, actor); err != nil {
			return err
		}

		now := time.Now()
		update := txClient.SupportIssue.UpdateOneID(issueID).SetStatus(nextStatus)
		switch nextStatus {
		case service.SupportIssueStatusResolved:
			update = update.SetResolvedAt(now).SetLockedAt(now).SetResolvedByUserID(actorUserID)
		case service.SupportIssueStatusClosed:
			update = update.SetLockedAt(now).ClearResolvedAt().ClearResolvedByUserID()
		default:
			update = update.ClearResolvedAt().ClearResolvedByUserID().ClearLockedAt()
		}

		updated, err := update.Save(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
		}
		out = supportIssueEntityToService(updated)

		if event.EventType == "" {
			event.EventType = service.SupportIssueEventStatusChanged
		}
		if event.FromStatus == nil {
			from := current.Status
			event.FromStatus = &from
		}
		if event.ToStatus == nil {
			to := nextStatus
			event.ToStatus = &to
		}
		event.IssueID = issueID
		if event.ActorUserID == nil && actorUserID > 0 {
			event.ActorUserID = &actorUserID
		}
		_, err = createSupportIssueEvent(txCtx, txClient, event)
		return err
	})
	return out, err
}

func (r *supportIssueRepository) HideComment(
	ctx context.Context,
	input service.HideSupportIssueCommentInput,
	event service.SupportIssueEvent,
) error {
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		issue, err := txClient.SupportIssue.Query().
			Where(supportissue.IDEQ(input.IssueID)).
			Only(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
		}

		comment, err := txClient.SupportIssueComment.Query().
			Where(
				supportissuecomment.IDEQ(input.CommentID),
				supportissuecomment.IssueIDEQ(input.IssueID),
			).
			Only(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueCommentNotFound, nil)
		}

		now := time.Now()
		if _, err := txClient.SupportIssueComment.UpdateOneID(comment.ID).
			SetHiddenAt(now).
			SetHiddenByUserID(input.HiddenByUserID).
			SetHideReason(input.HideReason).
			Save(txCtx); err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueCommentNotFound, nil)
		}

		if comment.HiddenAt == nil {
			commentCount := issue.CommentCount - 1
			if commentCount < 0 {
				commentCount = 0
			}
			if _, err := txClient.SupportIssue.UpdateOneID(issue.ID).
				SetCommentCount(commentCount).
				SetHiddenCommentCount(issue.HiddenCommentCount + 1).
				Save(txCtx); err != nil {
				return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
			}
		}

		event.IssueID = issue.ID
		if event.EventType == "" {
			event.EventType = service.SupportIssueEventCommentHidden
		}
		if event.ActorUserID == nil && input.HiddenByUserID > 0 {
			event.ActorUserID = &input.HiddenByUserID
		}
		_, err = createSupportIssueEvent(txCtx, txClient, event)
		return err
	})
}

func (r *supportIssueRepository) HideIssue(
	ctx context.Context,
	input service.HideSupportIssueInput,
	event service.SupportIssueEvent,
) (*service.SupportIssue, error) {
	var out *service.SupportIssue
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		now := time.Now()
		update := txClient.SupportIssue.UpdateOneID(input.IssueID).
			SetHiddenAt(now).
			SetHiddenByUserID(input.HiddenByUserID).
			SetHideReason(input.HideReason)
		row, err := update.Save(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
		}
		out = supportIssueEntityToService(row)

		event.IssueID = input.IssueID
		if event.EventType == "" {
			event.EventType = service.SupportIssueEventIssueHidden
		}
		if event.ActorUserID == nil && input.HiddenByUserID > 0 {
			event.ActorUserID = &input.HiddenByUserID
		}
		_, err = createSupportIssueEvent(txCtx, txClient, event)
		return err
	})
	return out, err
}

func (r *supportIssueRepository) RestoreIssue(
	ctx context.Context,
	issueID int64,
	actorUserID int64,
	event service.SupportIssueEvent,
) (*service.SupportIssue, error) {
	var out *service.SupportIssue
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		row, err := txClient.SupportIssue.UpdateOneID(issueID).
			ClearHiddenAt().
			ClearHiddenByUserID().
			SetHideReason("").
			Save(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
		}
		out = supportIssueEntityToService(row)

		event.IssueID = issueID
		if event.EventType == "" {
			event.EventType = service.SupportIssueEventIssueRestored
		}
		if event.ActorUserID == nil && actorUserID > 0 {
			event.ActorUserID = &actorUserID
		}
		_, err = createSupportIssueEvent(txCtx, txClient, event)
		return err
	})
	return out, err
}

func (r *supportIssueRepository) PinIssue(
	ctx context.Context,
	input service.PinSupportIssueInput,
	event service.SupportIssueEvent,
) (*service.SupportIssue, error) {
	var out *service.SupportIssue
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		now := time.Now()
		row, err := txClient.SupportIssue.UpdateOneID(input.IssueID).
			SetPinnedAt(now).
			SetPinnedByUserID(input.PinnedByUserID).
			SetPinnedReason(strings.TrimSpace(input.Reason)).
			Save(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
		}
		out = supportIssueEntityToService(row)

		event.IssueID = input.IssueID
		if event.EventType == "" {
			event.EventType = service.SupportIssueEventIssuePinned
		}
		if event.ActorUserID == nil && input.PinnedByUserID > 0 {
			event.ActorUserID = &input.PinnedByUserID
		}
		_, err = createSupportIssueEvent(txCtx, txClient, event)
		return err
	})
	return out, err
}

func (r *supportIssueRepository) UnpinIssue(
	ctx context.Context,
	issueID int64,
	actorUserID int64,
	event service.SupportIssueEvent,
) (*service.SupportIssue, error) {
	var out *service.SupportIssue
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		row, err := txClient.SupportIssue.UpdateOneID(issueID).
			ClearPinnedAt().
			ClearPinnedByUserID().
			SetPinnedReason("").
			Save(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
		}
		out = supportIssueEntityToService(row)

		event.IssueID = issueID
		if event.EventType == "" {
			event.EventType = service.SupportIssueEventIssueUnpinned
		}
		if event.ActorUserID == nil && actorUserID > 0 {
			event.ActorUserID = &actorUserID
		}
		_, err = createSupportIssueEvent(txCtx, txClient, event)
		return err
	})
	return out, err
}

func (r *supportIssueRepository) SetSolutionComment(
	ctx context.Context,
	issueID int64,
	commentID int64,
	actorUserID int64,
	event service.SupportIssueEvent,
) (*service.SupportIssue, error) {
	var out *service.SupportIssue
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		_, err := txClient.SupportIssueComment.Query().
			Where(
				supportissuecomment.IDEQ(commentID),
				supportissuecomment.IssueIDEQ(issueID),
				supportissuecomment.HiddenAtIsNil(),
			).
			Only(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueCommentNotFound, nil)
		}

		row, err := txClient.SupportIssue.UpdateOneID(issueID).
			SetSolutionCommentID(commentID).
			Save(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
		}
		out = supportIssueEntityToService(row)

		event.IssueID = issueID
		if event.EventType == "" {
			event.EventType = service.SupportIssueEventSolutionMarked
		}
		if event.ActorUserID == nil && actorUserID > 0 {
			event.ActorUserID = &actorUserID
		}
		_, err = createSupportIssueEvent(txCtx, txClient, event)
		return err
	})
	return out, err
}

func (r *supportIssueRepository) ClearSolutionComment(
	ctx context.Context,
	issueID int64,
	actorUserID int64,
	event service.SupportIssueEvent,
) (*service.SupportIssue, error) {
	var out *service.SupportIssue
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		row, err := txClient.SupportIssue.UpdateOneID(issueID).
			ClearSolutionCommentID().
			Save(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
		}
		out = supportIssueEntityToService(row)

		event.IssueID = issueID
		if event.EventType == "" {
			event.EventType = service.SupportIssueEventSolutionCleared
		}
		if event.ActorUserID == nil && actorUserID > 0 {
			event.ActorUserID = &actorUserID
		}
		_, err = createSupportIssueEvent(txCtx, txClient, event)
		return err
	})
	return out, err
}

func (r *supportIssueRepository) SetRelatedIssue(
	ctx context.Context,
	input service.SetRelatedSupportIssueInput,
	event service.SupportIssueEvent,
) (*service.SupportIssue, error) {
	var out *service.SupportIssue
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if input.IssueID <= 0 || input.RelatedIssueID <= 0 || input.IssueID == input.RelatedIssueID {
			return service.ErrSupportIssueInvalidInput
		}
		targetExists, err := txClient.SupportIssue.Query().
			Where(
				supportissue.IDEQ(input.RelatedIssueID),
				supportissue.HiddenAtIsNil(),
				supportissue.StatusEQ(service.SupportIssueStatusResolved),
			).
			Exist(txCtx)
		if err != nil {
			return err
		}
		if !targetExists {
			return service.ErrSupportIssueInvalidInput
		}

		row, err := txClient.SupportIssue.UpdateOneID(input.IssueID).
			SetRelatedIssueID(input.RelatedIssueID).
			SetRelatedIssueReason(strings.TrimSpace(input.Reason)).
			Save(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
		}
		out = supportIssueEntityToService(row)

		event.IssueID = input.IssueID
		if event.EventType == "" {
			event.EventType = service.SupportIssueEventRelatedSet
		}
		if event.ActorUserID == nil && input.ActorUserID > 0 {
			event.ActorUserID = &input.ActorUserID
		}
		_, err = createSupportIssueEvent(txCtx, txClient, event)
		return err
	})
	return out, err
}

func (r *supportIssueRepository) ClearRelatedIssue(
	ctx context.Context,
	issueID int64,
	actorUserID int64,
	event service.SupportIssueEvent,
) (*service.SupportIssue, error) {
	var out *service.SupportIssue
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		row, err := txClient.SupportIssue.UpdateOneID(issueID).
			ClearRelatedIssueID().
			SetRelatedIssueReason("").
			Save(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
		}
		out = supportIssueEntityToService(row)

		event.IssueID = issueID
		if event.EventType == "" {
			event.EventType = service.SupportIssueEventRelatedCleared
		}
		if event.ActorUserID == nil && actorUserID > 0 {
			event.ActorUserID = &actorUserID
		}
		_, err = createSupportIssueEvent(txCtx, txClient, event)
		return err
	})
	return out, err
}

func (r *supportIssueRepository) HideAttachment(
	ctx context.Context,
	input service.HideSupportIssueAttachmentInput,
	event service.SupportIssueEvent,
) error {
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		issue, err := txClient.SupportIssue.Query().
			Where(supportissue.IDEQ(input.IssueID)).
			Only(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
		}

		attachment, err := txClient.SupportIssueAttachment.Query().
			Where(
				supportissueattachment.IDEQ(input.AttachmentID),
				supportissueattachment.IssueIDEQ(input.IssueID),
			).
			Only(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueAttachmentNotFound, nil)
		}

		now := time.Now()
		if _, err := txClient.SupportIssueAttachment.UpdateOneID(attachment.ID).
			SetVisibility(service.SupportIssueAttachmentVisibilityHidden).
			SetHiddenAt(now).
			SetHiddenByUserID(input.HiddenByUserID).
			Save(txCtx); err != nil {
			return translatePersistenceError(err, service.ErrSupportIssueAttachmentNotFound, nil)
		}

		if attachment.Visibility != service.SupportIssueAttachmentVisibilityHidden {
			attachmentCount := issue.AttachmentCount - 1
			if attachmentCount < 0 {
				attachmentCount = 0
			}
			if _, err := txClient.SupportIssue.UpdateOneID(issue.ID).
				SetAttachmentCount(attachmentCount).
				Save(txCtx); err != nil {
				return translatePersistenceError(err, service.ErrSupportIssueNotFound, nil)
			}
		}

		event.IssueID = issue.ID
		if event.EventType == "" {
			event.EventType = service.SupportIssueEventAttachmentHidden
		}
		if event.ActorUserID == nil && input.HiddenByUserID > 0 {
			event.ActorUserID = &input.HiddenByUserID
		}
		_, err = createSupportIssueEvent(txCtx, txClient, event)
		return err
	})
}

func (r *supportIssueRepository) ListEvents(ctx context.Context, issueID int64) ([]service.SupportIssueEvent, error) {
	client := clientFromContext(ctx, r.client)
	exists, err := client.SupportIssue.Query().
		Where(supportissue.IDEQ(issueID)).
		Exist(ctx)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, service.ErrSupportIssueNotFound
	}

	rows, err := client.SupportIssueEvent.Query().
		Where(supportissueevent.IssueIDEQ(issueID)).
		Order(dbent.Desc(supportissueevent.FieldCreatedAt), dbent.Desc(supportissueevent.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return supportIssueEventEntitiesToService(rows), nil
}

func (r *supportIssueRepository) withTx(
	ctx context.Context,
	fn func(txCtx context.Context, txClient *dbent.Client) error,
) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx, tx.Client())
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	txCtx := dbent.NewTxContext(ctx, tx)

	if err := fn(txCtx, tx.Client()); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("%w; rollback failed: %v", err, rbErr)
		}
		return err
	}
	return tx.Commit()
}

func buildSupportIssueCreate(
	client *dbent.Client,
	issue *service.SupportIssue,
	attachments []service.SupportIssueAttachment,
) *dbent.SupportIssueCreate {
	status := defaultString(issue.Status, service.SupportIssueStatusOpen)
	category := defaultString(issue.Category, service.SupportIssueCategoryOther)
	severity := defaultString(issue.Severity, service.SupportIssueSeverityQuestion)
	language := defaultString(issue.ScreenshotLanguage, service.SupportIssueScreenshotLanguageUnknown)
	publicID := issue.PublicID
	if publicID == "" {
		publicID = fmt.Sprintf("PENDING-%d", time.Now().UnixNano())
	}

	builder := client.SupportIssue.Create().
		SetPublicID(publicID).
		SetTitle(issue.Title).
		SetDescription(issue.Description).
		SetAccountEmail(issue.AccountEmail).
		SetAccountEmailNormalized(issue.AccountEmailNormalized).
		SetAccountEmailMasked(issue.AccountEmailMasked).
		SetOccurredAt(issue.OccurredAt).
		SetScreenshotText(issue.ScreenshotText).
		SetScreenshotLanguage(language).
		SetCategory(category).
		SetSeverity(severity).
		SetStatus(status).
		SetModelName(issue.ModelName).
		SetClientName(issue.ClientName).
		SetErrorCode(issue.ErrorCode).
		SetAPIKeySuffix(issue.APIKeySuffix).
		SetAttachmentCount(visibleSupportIssueAttachmentCount(attachments)).
		SetSearchText(issue.SearchText)

	if issue.HTTPStatus != nil {
		builder.SetHTTPStatus(*issue.HTTPStatus)
	}
	if issue.CreatedByUserID != nil {
		builder.SetCreatedByUserID(*issue.CreatedByUserID)
	}
	if issue.ResolvedByUserID != nil {
		builder.SetResolvedByUserID(*issue.ResolvedByUserID)
	}
	if issue.ResolvedAt != nil {
		builder.SetResolvedAt(*issue.ResolvedAt)
	}
	if issue.LockedAt != nil {
		builder.SetLockedAt(*issue.LockedAt)
	}
	if issue.LastCommentAt != nil {
		builder.SetLastCommentAt(*issue.LastCommentAt)
	}
	if issue.CommentCount > 0 {
		builder.SetCommentCount(issue.CommentCount)
	}
	if issue.HiddenCommentCount > 0 {
		builder.SetHiddenCommentCount(issue.HiddenCommentCount)
	}
	if issue.PinnedAt != nil {
		builder.SetPinnedAt(*issue.PinnedAt)
	}
	if issue.PinnedByUserID != nil {
		builder.SetPinnedByUserID(*issue.PinnedByUserID)
	}
	if strings.TrimSpace(issue.PinnedReason) != "" {
		builder.SetPinnedReason(strings.TrimSpace(issue.PinnedReason))
	}
	if issue.SolutionCommentID != nil {
		builder.SetSolutionCommentID(*issue.SolutionCommentID)
	}
	if issue.RelatedIssueID != nil {
		builder.SetRelatedIssueID(*issue.RelatedIssueID)
	}
	if strings.TrimSpace(issue.RelatedIssueReason) != "" {
		builder.SetRelatedIssueReason(strings.TrimSpace(issue.RelatedIssueReason))
	}

	return builder
}

func createSupportIssueComment(
	ctx context.Context,
	client *dbent.Client,
	comment service.SupportIssueComment,
) (*dbent.SupportIssueComment, error) {
	builder := client.SupportIssueComment.Create().
		SetIssueID(comment.IssueID).
		SetAuthorRole(comment.AuthorRole).
		SetContent(comment.Content).
		SetHideReason(comment.HideReason)

	if comment.AuthorUserID != nil {
		builder.SetAuthorUserID(*comment.AuthorUserID)
	}
	if comment.HiddenAt != nil {
		builder.SetHiddenAt(*comment.HiddenAt)
	}
	if comment.HiddenByUserID != nil {
		builder.SetHiddenByUserID(*comment.HiddenByUserID)
	}
	if comment.RelatedIssueID != nil {
		builder.SetRelatedIssueID(*comment.RelatedIssueID)
	}
	return builder.Save(ctx)
}

func createSupportIssueAttachment(
	ctx context.Context,
	client *dbent.Client,
	attachment service.SupportIssueAttachment,
) (*dbent.SupportIssueAttachment, error) {
	visibility := defaultString(attachment.Visibility, service.SupportIssueAttachmentVisibilityPublic)
	builder := client.SupportIssueAttachment.Create().
		SetFilePath(attachment.FilePath).
		SetFileURL(attachment.FileURL).
		SetFileName(attachment.FileName).
		SetMimeType(attachment.MimeType).
		SetSizeBytes(attachment.SizeBytes).
		SetOcrText(attachment.OCRText).
		SetVisibility(visibility)

	if attachment.IssueID > 0 {
		builder.SetIssueID(attachment.IssueID)
	}
	if attachment.UploadedByUserID != nil {
		builder.SetUploadedByUserID(*attachment.UploadedByUserID)
	}
	if attachment.HiddenAt != nil {
		builder.SetHiddenAt(*attachment.HiddenAt)
	}
	if attachment.HiddenByUserID != nil {
		builder.SetHiddenByUserID(*attachment.HiddenByUserID)
	}
	return builder.Save(ctx)
}

func bindSupportIssueAttachment(
	ctx context.Context,
	client *dbent.Client,
	attachment service.SupportIssueAttachment,
	issueID int64,
) (*dbent.SupportIssueAttachment, error) {
	q := client.SupportIssueAttachment.Query().
		Where(
			supportissueattachment.IDEQ(attachment.ID),
			supportissueattachment.IssueIDIsNil(),
			supportissueattachment.VisibilityEQ(service.SupportIssueAttachmentVisibilityPublic),
			supportissueattachment.HiddenAtIsNil(),
		)
	if attachment.UploadedByUserID != nil {
		q = q.Where(supportissueattachment.UploadedByUserIDEQ(*attachment.UploadedByUserID))
	}
	current, err := q.Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSupportIssueAttachmentNotFound, nil)
	}
	return client.SupportIssueAttachment.UpdateOneID(current.ID).
		SetIssueID(issueID).
		Save(ctx)
}

func createSupportIssueEvent(
	ctx context.Context,
	client *dbent.Client,
	event service.SupportIssueEvent,
) (*dbent.SupportIssueEvent, error) {
	metadata := event.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	builder := client.SupportIssueEvent.Create().
		SetIssueID(event.IssueID).
		SetEventType(event.EventType).
		SetMetadata(metadata)

	if event.ActorUserID != nil {
		builder.SetActorUserID(*event.ActorUserID)
	}
	if event.FromStatus != nil {
		builder.SetFromStatus(*event.FromStatus)
	}
	if event.ToStatus != nil {
		builder.SetToStatus(*event.ToStatus)
	}
	return builder.Save(ctx)
}

func supportIssueQueryWithRelations(q *dbent.SupportIssueQuery, includeHidden bool) *dbent.SupportIssueQuery {
	return q.
		WithComments(func(cq *dbent.SupportIssueCommentQuery) {
			if !includeHidden {
				cq.Where(supportissuecomment.HiddenAtIsNil())
			}
			cq.Order(dbent.Asc(supportissuecomment.FieldCreatedAt), dbent.Asc(supportissuecomment.FieldID))
		}).
		WithAttachments(func(aq *dbent.SupportIssueAttachmentQuery) {
			if !includeHidden {
				aq.Where(
					supportissueattachment.VisibilityEQ(service.SupportIssueAttachmentVisibilityPublic),
					supportissueattachment.HiddenAtIsNil(),
				)
			}
			aq.Order(dbent.Asc(supportissueattachment.FieldID))
		})
}

func applySupportIssueListFilters(
	q *dbent.SupportIssueQuery,
	filters service.ListSupportIssueFilters,
) *dbent.SupportIssueQuery {
	if filters.Hidden != nil {
		if *filters.Hidden {
			q = q.Where(supportissue.HiddenAtNotNil())
		} else {
			q = q.Where(supportissue.HiddenAtIsNil())
		}
	} else if !filters.IncludeHidden {
		q = q.Where(supportissue.HiddenAtIsNil())
	}
	if filters.Status != "" {
		q = q.Where(supportissue.StatusEQ(filters.Status))
	}
	if filters.Category != "" {
		q = q.Where(supportissue.CategoryEQ(filters.Category))
	}
	if filters.Severity != "" {
		q = q.Where(supportissue.SeverityEQ(filters.Severity))
	}
	if filters.HasImage != nil {
		if *filters.HasImage {
			q = q.Where(supportissue.AttachmentCountGT(0))
		} else {
			q = q.Where(supportissue.AttachmentCountEQ(0))
		}
	}
	if filters.ActiveSince != nil {
		q = q.Where(supportissue.Or(
			supportissue.CreatedAtGTE(*filters.ActiveSince),
			supportissue.UpdatedAtGTE(*filters.ActiveSince),
			supportissue.LastCommentAtGTE(*filters.ActiveSince),
			supportissue.LastViewedAtGTE(*filters.ActiveSince),
		))
	}
	if filters.CreatedByUserID != nil {
		q = q.Where(supportissue.CreatedByUserIDEQ(*filters.CreatedByUserID))
	}
	return q
}

func applySupportIssueSearchQuery(
	q *dbent.SupportIssueQuery,
	parsed service.ParsedSupportIssueSearch,
) *dbent.SupportIssueQuery {
	if parsed.IssueID != nil {
		q = q.Where(supportissue.IDEQ(*parsed.IssueID))
	}
	if parsed.PublicID != "" {
		q = q.Where(supportissue.PublicIDEQ(parsed.PublicID))
	}
	if parsed.Email != "" {
		if parsed.EmailIsDomain {
			q = q.Where(supportissue.AccountEmailNormalizedContainsFold(parsed.Email))
		} else {
			q = q.Where(supportissue.AccountEmailNormalizedEQ(parsed.Email))
		}
	}
	if parsed.Status != "" {
		q = q.Where(supportissue.StatusEQ(parsed.Status))
	}
	if parsed.Category != "" {
		q = q.Where(supportissue.CategoryEQ(parsed.Category))
	}
	if parsed.Severity != "" {
		q = q.Where(supportissue.SeverityEQ(parsed.Severity))
	}
	if parsed.ModelName != "" {
		q = q.Where(supportissue.ModelNameContainsFold(parsed.ModelName))
	} else if parsed.Model != "" {
		q = q.Where(supportissue.ModelNameContainsFold(parsed.Model))
	}
	if parsed.ClientName != "" {
		q = q.Where(supportissue.ClientNameContainsFold(parsed.ClientName))
	} else if parsed.Client != "" {
		q = q.Where(supportissue.ClientNameContainsFold(parsed.Client))
	}
	if parsed.HTTPStatus != nil {
		q = q.Where(supportissue.HTTPStatusEQ(*parsed.HTTPStatus))
	}
	if parsed.ErrorCode != "" {
		q = q.Where(supportissue.ErrorCodeContainsFold(parsed.ErrorCode))
	}
	if parsed.APIKeySuffix != "" {
		q = q.Where(supportissue.APIKeySuffixEQ(parsed.APIKeySuffix))
	}
	if parsed.ScreenshotLanguage != "" {
		q = q.Where(supportissue.ScreenshotLanguageEQ(parsed.ScreenshotLanguage))
	} else if parsed.Language != "" {
		q = q.Where(supportissue.ScreenshotLanguageEQ(parsed.Language))
	}
	if parsed.HasImage != nil {
		if *parsed.HasImage {
			q = q.Where(supportissue.AttachmentCountGT(0))
		} else {
			q = q.Where(supportissue.AttachmentCountEQ(0))
		}
	}
	if parsed.OccurredFrom != nil {
		q = q.Where(supportissue.OccurredAtGTE(*parsed.OccurredFrom))
	}
	if parsed.OccurredTo != nil {
		q = q.Where(supportissue.OccurredAtLTE(*parsed.OccurredTo))
	}
	if parsed.TitlePhrase != "" {
		q = q.Where(supportissue.TitleContainsFold(parsed.TitlePhrase))
	}
	if parsed.ErrorPhrase != "" {
		q = q.Where(supportIssueTextPredicate(parsed.ErrorPhrase))
	}
	for _, phrase := range parsed.Phrases {
		if strings.TrimSpace(phrase) != "" {
			q = q.Where(supportIssueTextPredicate(phrase))
		}
	}
	for _, term := range parsed.Terms {
		if strings.TrimSpace(term) != "" {
			q = q.Where(supportIssueTextPredicate(term))
		}
	}
	return q
}

func supportIssueTextPredicate(value string) predicate.SupportIssue {
	return supportissue.Or(
		supportissue.PublicIDContainsFold(value),
		supportissue.TitleContainsFold(value),
		supportissue.DescriptionContainsFold(value),
		supportissue.ScreenshotTextContainsFold(value),
		supportissue.SearchTextContainsFold(value),
		supportissue.ModelNameContainsFold(value),
		supportissue.ClientNameContainsFold(value),
		supportissue.ErrorCodeContainsFold(value),
	)
}

func supportIssueListOrders(params pagination.PaginationParams) []func(*entsql.Selector) {
	field, sortOrder := supportIssueListOrder(params)
	pinnedFirst := func(s *entsql.Selector) {
		column := s.C(supportissue.FieldPinnedAt)
		s.OrderExpr(entsql.Expr(column+" IS NULL"), entsql.Expr(column+" DESC"))
	}
	if sortOrder == pagination.SortOrderAsc {
		if field == supportissue.FieldID {
			return []func(*entsql.Selector){pinnedFirst, dbent.Asc(field)}
		}
		return []func(*entsql.Selector){pinnedFirst, dbent.Asc(field), dbent.Asc(supportissue.FieldID)}
	}

	if field == supportissue.FieldID {
		return []func(*entsql.Selector){pinnedFirst, dbent.Desc(field)}
	}
	return []func(*entsql.Selector){pinnedFirst, dbent.Desc(field), dbent.Desc(supportissue.FieldID)}
}

func supportIssueListOrder(params pagination.PaginationParams) (string, string) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)
	switch sortBy {
	case "id":
		return supportissue.FieldID, sortOrder
	case "occurred_at":
		return supportissue.FieldOccurredAt, sortOrder
	case "created_at":
		return supportissue.FieldCreatedAt, sortOrder
	case "last_comment_at", "last_activity":
		return supportissue.FieldLastCommentAt, sortOrder
	case "last_viewed_at":
		return supportissue.FieldLastViewedAt, sortOrder
	case "comment_count", "replies", "reply_count":
		return supportissue.FieldCommentCount, sortOrder
	case "view_count", "views", "popular", "hot", "hot_24h":
		return supportissue.FieldViewCount, sortOrder
	case "pinned_at", "pinned":
		return supportissue.FieldPinnedAt, sortOrder
	case "status":
		return supportissue.FieldStatus, sortOrder
	case "category":
		return supportissue.FieldCategory, sortOrder
	case "severity":
		return supportissue.FieldSeverity, sortOrder
	case "", "updated_at":
		return supportissue.FieldUpdatedAt, sortOrder
	default:
		return supportissue.FieldUpdatedAt, pagination.SortOrderDesc
	}
}

func supportIssueEntityToService(row *dbent.SupportIssue) *service.SupportIssue {
	if row == nil {
		return nil
	}
	out := &service.SupportIssue{}
	applySupportIssueEntityToService(out, row)
	out.Comments = supportIssueCommentEntitiesToService(row.Edges.Comments)
	out.Attachments = supportIssueAttachmentEntitiesToService(row.Edges.Attachments)
	out.Events = supportIssueEventEntitiesToService(row.Edges.Events)
	return out
}

func (r *supportIssueRepository) populateSupportIssueReferences(
	ctx context.Context,
	client *dbent.Client,
	issue *service.SupportIssue,
	includeHidden bool,
) error {
	if issue == nil {
		return nil
	}

	if err := populateSupportIssueCommentAuthors(ctx, client, issue.Comments); err != nil {
		return err
	}

	if issue.SolutionCommentID != nil {
		for i := range issue.Comments {
			if issue.Comments[i].ID == *issue.SolutionCommentID {
				comment := issue.Comments[i]
				issue.SolutionComment = &comment
				break
			}
		}
	}

	ids := make([]int64, 0, len(issue.Comments)+1)
	if issue.RelatedIssueID != nil {
		ids = append(ids, *issue.RelatedIssueID)
	}
	for i := range issue.Comments {
		if issue.Comments[i].RelatedIssueID != nil {
			ids = append(ids, *issue.Comments[i].RelatedIssueID)
		}
	}
	refs, err := supportIssueReferencesByID(ctx, client, ids, includeHidden)
	if err != nil {
		return err
	}
	if issue.RelatedIssueID != nil {
		issue.RelatedIssue = refs[*issue.RelatedIssueID]
	}
	for i := range issue.Comments {
		if issue.Comments[i].RelatedIssueID != nil {
			issue.Comments[i].RelatedIssue = refs[*issue.Comments[i].RelatedIssueID]
		}
	}
	if issue.SolutionComment != nil && issue.SolutionComment.RelatedIssueID != nil {
		issue.SolutionComment.RelatedIssue = refs[*issue.SolutionComment.RelatedIssueID]
	}
	return nil
}

func populateSupportIssueCommentAuthors(
	ctx context.Context,
	client *dbent.Client,
	comments []service.SupportIssueComment,
) error {
	if len(comments) == 0 {
		return nil
	}
	seen := map[int64]struct{}{}
	ids := make([]int64, 0, len(comments))
	for i := range comments {
		if comments[i].AuthorUserID == nil || *comments[i].AuthorUserID <= 0 {
			continue
		}
		id := *comments[i].AuthorUserID
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}

	rows, err := client.User.Query().
		Where(user.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return err
	}
	displayNames := make(map[int64]string, len(rows))
	for _, row := range rows {
		displayNames[row.ID] = supportIssueCommentAuthorDisplayName(row.Username, row.Email)
	}
	for i := range comments {
		if comments[i].AuthorUserID == nil {
			continue
		}
		if displayName := displayNames[*comments[i].AuthorUserID]; displayName != "" {
			comments[i].AuthorDisplayName = displayName
		}
	}
	return nil
}

func supportIssueCommentAuthorDisplayName(username string, email string) string {
	if displayName := strings.TrimSpace(username); displayName != "" {
		return displayName
	}
	return maskSupportIssueDisplayEmail(email)
}

func maskSupportIssueDisplayEmail(email string) string {
	normalized := strings.TrimSpace(strings.ToLower(email))
	parts := strings.SplitN(normalized, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return normalized
	}

	localRunes := []rune(parts[0])
	if len(localRunes) <= 4 {
		return fmt.Sprintf("%c***@%s", localRunes[0], parts[1])
	}
	maskRunes := 3
	head := (len(localRunes) - maskRunes + 1) / 2
	tail := len(localRunes) - maskRunes - head
	if head < 1 {
		head = 1
	}
	if tail < 1 {
		tail = 1
	}
	return fmt.Sprintf("%s***%s@%s", string(localRunes[:head]), string(localRunes[len(localRunes)-tail:]), parts[1])
}

func supportIssueReferencesByID(
	ctx context.Context,
	client *dbent.Client,
	ids []int64,
	includeHidden bool,
) (map[int64]*service.SupportIssueReference, error) {
	out := map[int64]*service.SupportIssueReference{}
	if len(ids) == 0 {
		return out, nil
	}
	seen := make(map[int64]struct{}, len(ids))
	unique := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return out, nil
	}

	q := client.SupportIssue.Query().Where(supportissue.IDIn(unique...))
	if !includeHidden {
		q = q.Where(supportissue.HiddenAtIsNil())
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = &service.SupportIssueReference{
			ID:         row.ID,
			PublicID:   row.PublicID,
			Title:      row.Title,
			Status:     row.Status,
			ResolvedAt: row.ResolvedAt,
		}
	}
	return out, nil
}

func applySupportIssueEntityToService(dst *service.SupportIssue, row *dbent.SupportIssue) {
	if dst == nil || row == nil {
		return
	}
	dst.ID = row.ID
	dst.PublicID = row.PublicID
	dst.Title = row.Title
	dst.Description = row.Description
	dst.AccountEmail = row.AccountEmail
	dst.AccountEmailNormalized = row.AccountEmailNormalized
	dst.AccountEmailMasked = row.AccountEmailMasked
	dst.OccurredAt = row.OccurredAt
	dst.ScreenshotText = row.ScreenshotText
	dst.ScreenshotLanguage = row.ScreenshotLanguage
	dst.Category = row.Category
	dst.Severity = row.Severity
	dst.Status = row.Status
	dst.ModelName = row.ModelName
	dst.ClientName = row.ClientName
	dst.HTTPStatus = row.HTTPStatus
	dst.ErrorCode = row.ErrorCode
	dst.APIKeySuffix = row.APIKeySuffix
	dst.CreatedByUserID = row.CreatedByUserID
	dst.ResolvedByUserID = row.ResolvedByUserID
	dst.ResolvedAt = row.ResolvedAt
	dst.LockedAt = row.LockedAt
	dst.HiddenAt = row.HiddenAt
	dst.HiddenByUserID = row.HiddenByUserID
	dst.HideReason = row.HideReason
	dst.PinnedAt = row.PinnedAt
	dst.PinnedByUserID = row.PinnedByUserID
	dst.PinnedReason = row.PinnedReason
	dst.SolutionCommentID = row.SolutionCommentID
	dst.RelatedIssueID = row.RelatedIssueID
	dst.RelatedIssueReason = row.RelatedIssueReason
	dst.LastCommentAt = row.LastCommentAt
	dst.LastViewedAt = row.LastViewedAt
	dst.CommentCount = row.CommentCount
	dst.HiddenCommentCount = row.HiddenCommentCount
	dst.AttachmentCount = row.AttachmentCount
	dst.ViewCount = row.ViewCount
	dst.SearchText = row.SearchText
	dst.CreatedAt = row.CreatedAt
	dst.UpdatedAt = row.UpdatedAt
}

func supportIssueEntitiesToService(rows []*dbent.SupportIssue) []service.SupportIssue {
	out := make([]service.SupportIssue, 0, len(rows))
	for _, row := range rows {
		if item := supportIssueEntityToService(row); item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func populateSupportIssueViewerState(ctx context.Context, client *dbent.Client, items []service.SupportIssue, viewerUserID int64) error {
	if viewerUserID <= 0 || len(items) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(items))
	for i := range items {
		if items[i].ID > 0 {
			ids = append(ids, items[i].ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	lastViewed, err := supportIssueViewerLastViewedAt(ctx, client, viewerUserID, ids)
	if err != nil {
		return err
	}
	lastActivity, err := supportIssueViewerLastExternalActivityAt(ctx, client, viewerUserID, ids)
	if err != nil {
		return err
	}

	for i := range items {
		issue := &items[i]
		if viewedAt, ok := lastViewed[issue.ID]; ok {
			value := viewedAt
			issue.ViewerLastViewedAt = &value
		}
		if activityAt, ok := lastActivity[issue.ID]; ok {
			value := activityAt
			issue.ViewerLastActivityAt = &value
			issue.HasUnreadActivity = issue.ViewerLastViewedAt == nil || activityAt.After(*issue.ViewerLastViewedAt)
		}
		issue.ViewerAttentionReason = supportIssueViewerAttentionReason(issue)
	}
	return nil
}

func supportIssueViewerLastViewedAt(ctx context.Context, client *dbent.Client, viewerUserID int64, issueIDs []int64) (map[int64]time.Time, error) {
	rows, err := client.SupportIssueView.Query().
		Where(
			supportissueview.IssueIDIn(issueIDs...),
			supportissueview.ViewerUserIDEQ(viewerUserID),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make(map[int64]time.Time, len(rows))
	for _, row := range rows {
		if current, ok := out[row.IssueID]; !ok || row.ViewedAt.After(current) {
			out[row.IssueID] = row.ViewedAt
		}
	}
	return out, nil
}

func supportIssueViewerLastExternalActivityAt(ctx context.Context, client *dbent.Client, viewerUserID int64, issueIDs []int64) (map[int64]time.Time, error) {
	rows, err := client.SupportIssueEvent.Query().
		Where(
			supportissueevent.IssueIDIn(issueIDs...),
			supportissueevent.EventTypeNEQ(service.SupportIssueEventCreated),
			supportissueevent.Or(
				supportissueevent.ActorUserIDIsNil(),
				supportissueevent.ActorUserIDNEQ(viewerUserID),
			),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make(map[int64]time.Time, len(rows))
	for _, row := range rows {
		if current, ok := out[row.IssueID]; !ok || row.CreatedAt.After(current) {
			out[row.IssueID] = row.CreatedAt
		}
	}
	return out, nil
}

func supportIssueViewerAttentionReason(issue *service.SupportIssue) string {
	if issue == nil {
		return ""
	}
	if issue.Status == service.SupportIssueStatusNeedsInfo {
		return "needs_info"
	}
	if !issue.HasUnreadActivity {
		return ""
	}
	if issue.SolutionCommentID != nil {
		return "solution"
	}
	if issue.RelatedIssueID != nil {
		return "related_solved"
	}
	if issue.Status == service.SupportIssueStatusResolved {
		return "resolved"
	}
	return "new_activity"
}

func supportIssueCommentEntityToService(row *dbent.SupportIssueComment) *service.SupportIssueComment {
	if row == nil {
		return nil
	}
	out := &service.SupportIssueComment{}
	applySupportIssueCommentEntityToService(out, row)
	return out
}

func applySupportIssueCommentEntityToService(dst *service.SupportIssueComment, row *dbent.SupportIssueComment) {
	if dst == nil || row == nil {
		return
	}
	dst.ID = row.ID
	dst.IssueID = row.IssueID
	dst.AuthorUserID = row.AuthorUserID
	dst.AuthorRole = row.AuthorRole
	dst.Content = row.Content
	dst.RelatedIssueID = row.RelatedIssueID
	dst.HiddenAt = row.HiddenAt
	dst.HiddenByUserID = row.HiddenByUserID
	dst.HideReason = row.HideReason
	dst.CreatedAt = row.CreatedAt
	dst.UpdatedAt = row.UpdatedAt
}

func supportIssueCommentEntitiesToService(rows []*dbent.SupportIssueComment) []service.SupportIssueComment {
	out := make([]service.SupportIssueComment, 0, len(rows))
	for _, row := range rows {
		if item := supportIssueCommentEntityToService(row); item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func supportIssueAttachmentEntityToService(row *dbent.SupportIssueAttachment) *service.SupportIssueAttachment {
	if row == nil {
		return nil
	}
	out := &service.SupportIssueAttachment{
		ID:               row.ID,
		UploadedByUserID: row.UploadedByUserID,
		FilePath:         row.FilePath,
		FileURL:          row.FileURL,
		FileName:         row.FileName,
		MimeType:         row.MimeType,
		SizeBytes:        row.SizeBytes,
		OCRText:          row.OcrText,
		Visibility:       row.Visibility,
		HiddenAt:         row.HiddenAt,
		HiddenByUserID:   row.HiddenByUserID,
		CreatedAt:        row.CreatedAt,
	}
	if row.IssueID != nil {
		out.IssueID = *row.IssueID
	}
	return out
}

func applySupportIssueAttachmentEntityToService(dst *service.SupportIssueAttachment, row *dbent.SupportIssueAttachment) {
	if dst == nil || row == nil {
		return
	}
	if item := supportIssueAttachmentEntityToService(row); item != nil {
		*dst = *item
	}
}

func supportIssueAttachmentEntitiesToService(rows []*dbent.SupportIssueAttachment) []service.SupportIssueAttachment {
	out := make([]service.SupportIssueAttachment, 0, len(rows))
	for _, row := range rows {
		if item := supportIssueAttachmentEntityToService(row); item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func supportIssueEventEntityToService(row *dbent.SupportIssueEvent) *service.SupportIssueEvent {
	if row == nil {
		return nil
	}
	return &service.SupportIssueEvent{
		ID:          row.ID,
		IssueID:     row.IssueID,
		ActorUserID: row.ActorUserID,
		EventType:   row.EventType,
		FromStatus:  row.FromStatus,
		ToStatus:    row.ToStatus,
		Metadata:    row.Metadata,
		CreatedAt:   row.CreatedAt,
	}
}

func supportIssueEventEntitiesToService(rows []*dbent.SupportIssueEvent) []service.SupportIssueEvent {
	out := make([]service.SupportIssueEvent, 0, len(rows))
	for _, row := range rows {
		if item := supportIssueEventEntityToService(row); item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func visibleSupportIssueAttachmentCount(attachments []service.SupportIssueAttachment) int {
	count := 0
	for _, attachment := range attachments {
		if attachment.Visibility == "" || attachment.Visibility == service.SupportIssueAttachmentVisibilityPublic {
			count++
		}
	}
	return count
}

func supportIssueAttachmentFileURL(id int64) string {
	return fmt.Sprintf("/api/v1/issues/attachments/%d/file", id)
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func supportIssueViewerHash(viewer service.SupportIssueViewer) string {
	parts := []string{
		fmt.Sprintf("user:%d", viewer.UserID),
		"ip:" + strings.TrimSpace(viewer.IP),
		"ua:" + strings.TrimSpace(viewer.UserAgent),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}
