package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/supportissue"
	"github.com/Wei-Shaw/sub2api/ent/supportissueattachment"
	"github.com/Wei-Shaw/sub2api/ent/supportissuecomment"
	"github.com/Wei-Shaw/sub2api/ent/supportissueevent"
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
	return supportIssueEntityToService(row), nil
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

	return supportIssueEntitiesToService(rows), paginationResultFromTotal(int64(total), params), nil
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

	return supportIssueEntitiesToService(rows), paginationResultFromTotal(int64(total), params), nil
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
				aq.Where(supportissueattachment.VisibilityEQ(service.SupportIssueAttachmentVisibilityPublic))
			}
			aq.Order(dbent.Asc(supportissueattachment.FieldID))
		})
}

func applySupportIssueListFilters(
	q *dbent.SupportIssueQuery,
	filters service.ListSupportIssueFilters,
) *dbent.SupportIssueQuery {
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
	if sortOrder == pagination.SortOrderAsc {
		if field == supportissue.FieldID {
			return []func(*entsql.Selector){dbent.Asc(field)}
		}
		return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(supportissue.FieldID)}
	}

	if field == supportissue.FieldID {
		return []func(*entsql.Selector){dbent.Desc(field)}
	}
	return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(supportissue.FieldID)}
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
	dst.LastCommentAt = row.LastCommentAt
	dst.CommentCount = row.CommentCount
	dst.HiddenCommentCount = row.HiddenCommentCount
	dst.AttachmentCount = row.AttachmentCount
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
