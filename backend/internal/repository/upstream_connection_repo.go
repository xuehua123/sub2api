package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/upstreamaccountbinding"
	"github.com/Wei-Shaw/sub2api/ent/upstreamconnection"
	"github.com/Wei-Shaw/sub2api/ent/upstreamgroup"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type upstreamConnectionRepository struct {
	client *dbent.Client
}

func NewUpstreamConnectionRepository(client *dbent.Client) service.UpstreamConnectionRepository {
	return &upstreamConnectionRepository{client: client}
}

func (r *upstreamConnectionRepository) Create(ctx context.Context, connection *service.UpstreamConnection) error {
	client := clientFromContext(ctx, r.client)
	builder := client.UpstreamConnection.Create().
		SetName(connection.Name).
		SetProvider(connection.Provider).
		SetAuthMode(connection.AuthMode).
		SetManagementBaseURL(connection.ManagementBaseURL).
		SetForwardingBaseURL(connection.ForwardingBaseURL).
		SetCredentialEncrypted(connection.CredentialEncrypted).
		SetCredentialFingerprint(connection.CredentialFingerprint).
		SetCredentialHint(connection.CredentialHint).
		SetRemoteUserID(connection.RemoteUserID).
		SetCapabilities(nonNilJSONMap(connection.Capabilities)).
		SetStatus(connection.Status).
		SetLastError(connection.LastError).
		SetSyncEnabled(connection.SyncEnabled).
		SetSyncIntervalSeconds(connection.SyncIntervalSeconds).
		SetSyncFailures(connection.SyncFailures).
		SetVersion(connection.Version).
		SetWalletCurrency(connection.WalletCurrency).
		SetWalletUnlimited(connection.WalletUnlimited).
		SetWalletSource(connection.WalletSource).
		SetWalletReliability(connection.WalletReliability).
		SetWalletRaw(nonNilJSONMap(connection.WalletRaw)).
		SetNillableProxyID(connection.ProxyID).
		SetNillableWalletAmount(connection.WalletAmount).
		SetNillableWalletUsd(connection.WalletUSD).
		SetNillableWalletObservedAt(connection.WalletObservedAt).
		SetNillableLastDiscoveredAt(connection.LastDiscoveredAt).
		SetNillableLastSyncedAt(connection.LastSyncedAt).
		SetNillableNextSyncAt(connection.NextSyncAt)
	if connection.LegacyMigrationKey != "" {
		builder = builder.SetLegacyMigrationKey(connection.LegacyMigrationKey)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return translateUpstreamConnectionPersistenceError(err)
	}
	applyEntUpstreamConnection(connection, created, false)
	return nil
}

func (r *upstreamConnectionRepository) GetByLegacyMigrationKey(ctx context.Context, key string) (*service.UpstreamConnection, error) {
	client := clientFromContext(ctx, r.client)
	row, err := client.UpstreamConnection.Query().
		Where(upstreamconnection.LegacyMigrationKeyEQ(strings.TrimSpace(key))).
		Only(ctx)
	if err != nil {
		return nil, translateUpstreamConnectionPersistenceError(err)
	}
	return entUpstreamConnectionToService(row, false), nil
}

func (r *upstreamConnectionRepository) GetByID(ctx context.Context, id int64) (*service.UpstreamConnection, error) {
	client := clientFromContext(ctx, r.client)
	row, err := client.UpstreamConnection.Query().
		Where(upstreamconnection.IDEQ(id)).
		WithGroups(func(query *dbent.UpstreamGroupQuery) {
			query.Order(dbent.Asc(upstreamgroup.FieldName), dbent.Asc(upstreamgroup.FieldID))
		}).
		WithAccountBindings(func(query *dbent.UpstreamAccountBindingQuery) {
			query.Order(dbent.Asc(upstreamaccountbinding.FieldAccountID), dbent.Asc(upstreamaccountbinding.FieldID))
		}).
		Only(ctx)
	if err != nil {
		return nil, translateUpstreamConnectionPersistenceError(err)
	}
	return entUpstreamConnectionToService(row, true), nil
}

func (r *upstreamConnectionRepository) List(ctx context.Context, params service.UpstreamConnectionListParams) ([]*service.UpstreamConnection, int64, error) {
	client := clientFromContext(ctx, r.client)
	query := client.UpstreamConnection.Query()
	if params.Provider != "" {
		query = query.Where(upstreamconnection.ProviderEQ(params.Provider))
	}
	if params.Status != "" {
		query = query.Where(upstreamconnection.StatusEQ(params.Status))
	}
	if search := strings.TrimSpace(params.Search); search != "" {
		query = query.Where(upstreamconnection.Or(
			upstreamconnection.NameContainsFold(search),
			upstreamconnection.ManagementBaseURLContainsFold(search),
			upstreamconnection.ForwardingBaseURLContainsFold(search),
			upstreamconnection.CredentialHintContainsFold(search),
			upstreamconnection.RemoteUserIDContainsFold(search),
		))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count upstream connections: %w", err)
	}
	rows, err := query.
		WithGroups(func(groupQuery *dbent.UpstreamGroupQuery) {
			groupQuery.Select(upstreamgroup.FieldID)
		}).
		WithAccountBindings(func(bindingQuery *dbent.UpstreamAccountBindingQuery) {
			bindingQuery.Select(upstreamaccountbinding.FieldID)
		}).
		Order(dbent.Desc(upstreamconnection.FieldID)).
		Offset((params.Page - 1) * params.PageSize).
		Limit(params.PageSize).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list upstream connections: %w", err)
	}
	items := make([]*service.UpstreamConnection, 0, len(rows))
	for _, row := range rows {
		items = append(items, entUpstreamConnectionToService(row, false))
	}
	return items, int64(total), nil
}

func (r *upstreamConnectionRepository) UpdateIfVersion(
	ctx context.Context,
	connection *service.UpstreamConnection,
	expectedVersion int64,
	resetBindings bool,
) (bool, error) {
	if existingTx := dbent.TxFromContext(ctx); existingTx != nil {
		return updateUpstreamConnectionWithClient(ctx, existingTx.Client(), connection, expectedVersion, resetBindings)
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return false, fmt.Errorf("begin upstream connection update: %w", err)
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	applied, err := updateUpstreamConnectionWithClient(txCtx, tx.Client(), connection, expectedVersion, resetBindings)
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit upstream connection update: %w", err)
	}
	return applied, nil
}

func updateUpstreamConnectionWithClient(
	ctx context.Context,
	client *dbent.Client,
	connection *service.UpstreamConnection,
	expectedVersion int64,
	resetBindings bool,
) (bool, error) {
	updater := client.UpstreamConnection.Update().
		Where(upstreamconnection.IDEQ(connection.ID), upstreamconnection.VersionEQ(expectedVersion)).
		SetName(connection.Name).
		SetProvider(connection.Provider).
		SetAuthMode(connection.AuthMode).
		SetManagementBaseURL(connection.ManagementBaseURL).
		SetForwardingBaseURL(connection.ForwardingBaseURL).
		SetCredentialEncrypted(connection.CredentialEncrypted).
		SetCredentialFingerprint(connection.CredentialFingerprint).
		SetCredentialHint(connection.CredentialHint).
		SetRemoteUserID(connection.RemoteUserID).
		SetCapabilities(nonNilJSONMap(connection.Capabilities)).
		SetStatus(connection.Status).
		SetLastError(connection.LastError).
		SetSyncEnabled(connection.SyncEnabled).
		SetSyncIntervalSeconds(connection.SyncIntervalSeconds).
		SetSyncFailures(connection.SyncFailures).
		SetVersion(connection.Version).
		SetWalletCurrency(connection.WalletCurrency).
		SetWalletUnlimited(connection.WalletUnlimited).
		SetWalletSource(connection.WalletSource).
		SetWalletReliability(connection.WalletReliability).
		SetWalletRaw(nonNilJSONMap(connection.WalletRaw))
	if connection.LegacyMigrationKey != "" {
		updater = updater.SetLegacyMigrationKey(connection.LegacyMigrationKey)
	} else {
		updater = updater.ClearLegacyMigrationKey()
	}

	if connection.ProxyID != nil {
		updater = updater.SetProxyID(*connection.ProxyID)
	} else {
		updater = updater.ClearProxyID()
	}
	if connection.WalletAmount != nil {
		updater = updater.SetWalletAmount(*connection.WalletAmount)
	} else {
		updater = updater.ClearWalletAmount()
	}
	if connection.WalletUSD != nil {
		updater = updater.SetWalletUsd(*connection.WalletUSD)
	} else {
		updater = updater.ClearWalletUsd()
	}
	if connection.WalletObservedAt != nil {
		updater = updater.SetWalletObservedAt(*connection.WalletObservedAt)
	} else {
		updater = updater.ClearWalletObservedAt()
	}
	if connection.LastDiscoveredAt != nil {
		updater = updater.SetLastDiscoveredAt(*connection.LastDiscoveredAt)
	} else {
		updater = updater.ClearLastDiscoveredAt()
	}
	if connection.LastSyncedAt != nil {
		updater = updater.SetLastSyncedAt(*connection.LastSyncedAt)
	} else {
		updater = updater.ClearLastSyncedAt()
	}
	if connection.NextSyncAt != nil {
		updater = updater.SetNextSyncAt(*connection.NextSyncAt)
	} else {
		updater = updater.ClearNextSyncAt()
	}

	affected, err := updater.Save(ctx)
	if err != nil {
		return false, translateUpstreamConnectionPersistenceError(err)
	}
	if affected == 0 {
		return false, nil
	}
	if resetBindings {
		if _, err := client.UpstreamGroup.Delete().
			Where(upstreamgroup.ConnectionIDEQ(connection.ID)).
			Exec(ctx); err != nil {
			return false, fmt.Errorf("clear stale upstream group snapshot: %w", err)
		}
		bindingUpdater := client.UpstreamAccountBinding.Update().
			Where(upstreamaccountbinding.ConnectionIDEQ(connection.ID)).
			SetRemoteTokenID("").
			SetRemoteTokenName("").
			SetResolutionKind(service.UpstreamBindingResolutionUnresolved).
			SetRemoteGroupID("").
			SetRemoteGroupName("").
			SetFallbackGroups([]string{}).
			ClearObservedMultiplier().
			SetConfidence("unknown").
			SetSource("").
			SetApplyPolicy(service.UpstreamBindingApplyObserveOnly).
			SetStatus(service.UpstreamBindingStatusPending).
			SetSyncFailures(0).
			SetLastError("").
			SetResolutionDetails(map[string]any{}).
			ClearObservedAt().
			ClearFreshUntil()
		if connection.SyncEnabled {
			bindingUpdater = bindingUpdater.SetNextSyncAt(time.Now().UTC())
		} else {
			bindingUpdater = bindingUpdater.ClearNextSyncAt()
		}
		if _, err := bindingUpdater.Save(ctx); err != nil {
			return false, fmt.Errorf("reset upstream account bindings: %w", err)
		}
	}
	updated, err := client.UpstreamConnection.Get(ctx, connection.ID)
	if err != nil {
		return false, translateUpstreamConnectionPersistenceError(err)
	}
	applyEntUpstreamConnection(connection, updated, false)
	return true, nil
}

func (r *upstreamConnectionRepository) DeleteIfUnbound(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	deleted, err := client.UpstreamConnection.Delete().Where(
		upstreamconnection.IDEQ(id),
		upstreamconnection.Not(upstreamconnection.HasAccountBindings()),
	).Exec(ctx)
	if err != nil {
		return translateUpstreamConnectionPersistenceError(err)
	}
	if deleted == 1 {
		return nil
	}
	exists, err := client.UpstreamConnection.Query().Where(upstreamconnection.IDEQ(id)).Exist(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return service.ErrUpstreamConnectionNotFound
	}
	return service.ErrUpstreamConnectionInUse
}

func (r *upstreamConnectionRepository) UpdateCredentialIfVersion(
	ctx context.Context,
	id, expectedVersion int64,
	update service.UpstreamConnectionCredentialPersistence,
) (bool, error) {
	client := clientFromContext(ctx, r.client)
	affected, err := client.UpstreamConnection.Update().
		Where(upstreamconnection.IDEQ(id), upstreamconnection.VersionEQ(expectedVersion)).
		SetCredentialEncrypted(update.CredentialEncrypted).
		SetCredentialFingerprint(update.CredentialFingerprint).
		SetCredentialHint(update.CredentialHint).
		SetVersion(update.Version).
		Save(ctx)
	if err != nil {
		return false, translateUpstreamConnectionPersistenceError(err)
	}
	return affected == 1, nil
}

func (r *upstreamConnectionRepository) FinalizeCredentialRefresh(
	ctx context.Context,
	id int64,
	expectedCiphertext, expectedProvider, expectedAuthMode, expectedManagementBaseURL string,
	update service.UpstreamConnectionCredentialPersistence,
) (bool, error) {
	client := clientFromContext(ctx, r.client)
	affected, err := client.UpstreamConnection.Update().
		Where(
			upstreamconnection.IDEQ(id),
			upstreamconnection.CredentialEncryptedEQ(expectedCiphertext),
			upstreamconnection.ProviderEQ(expectedProvider),
			upstreamconnection.AuthModeEQ(expectedAuthMode),
			upstreamconnection.ManagementBaseURLEQ(expectedManagementBaseURL),
		).
		SetCredentialEncrypted(update.CredentialEncrypted).
		SetCredentialFingerprint(update.CredentialFingerprint).
		SetCredentialHint(update.CredentialHint).
		AddVersion(1).
		Save(ctx)
	if err != nil {
		return false, translateUpstreamConnectionPersistenceError(err)
	}
	return affected == 1, nil
}

func (r *upstreamConnectionRepository) ApplyProbeSuccess(
	ctx context.Context,
	id, expectedVersion int64,
	update service.UpstreamConnectionProbePersistence,
) (bool, error) {
	if existingTx := dbent.TxFromContext(ctx); existingTx != nil {
		return applyUpstreamProbeSuccessWithClient(ctx, existingTx.Client(), id, expectedVersion, update)
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return false, fmt.Errorf("begin upstream probe transaction: %w", err)
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	applied, err := applyUpstreamProbeSuccessWithClient(txCtx, tx.Client(), id, expectedVersion, update)
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit upstream probe transaction: %w", err)
	}
	return applied, nil
}

func applyUpstreamProbeSuccessWithClient(
	ctx context.Context,
	client *dbent.Client,
	id, expectedVersion int64,
	update service.UpstreamConnectionProbePersistence,
) (bool, error) {
	updater := client.UpstreamConnection.Update().
		Where(upstreamconnection.IDEQ(id), upstreamconnection.VersionEQ(expectedVersion)).
		SetRemoteUserID(update.RemoteUserID).
		SetCapabilities(nonNilJSONMap(update.Capabilities)).
		SetStatus(update.Status).
		SetLastError(update.LastError).
		SetSyncFailures(update.SyncFailures).
		SetVersion(update.Version)
	if update.LastDiscoveredAt != nil {
		updater = updater.SetLastDiscoveredAt(*update.LastDiscoveredAt)
	}
	if update.LastSyncedAt != nil {
		updater = updater.SetLastSyncedAt(*update.LastSyncedAt)
	}
	if update.NextSyncAt != nil {
		updater = updater.SetNextSyncAt(*update.NextSyncAt)
	} else {
		updater = updater.ClearNextSyncAt()
	}
	if update.WalletObserved {
		updater = updater.
			SetWalletCurrency(update.WalletCurrency).
			SetWalletUnlimited(update.WalletUnlimited).
			SetWalletSource(update.WalletSource).
			SetWalletReliability(update.WalletReliability).
			SetWalletRaw(nonNilJSONMap(update.WalletRaw))
		if update.WalletAmount != nil {
			updater = updater.SetWalletAmount(*update.WalletAmount)
		} else {
			updater = updater.ClearWalletAmount()
		}
		if update.WalletUSD != nil {
			updater = updater.SetWalletUsd(*update.WalletUSD)
		} else {
			updater = updater.ClearWalletUsd()
		}
		if update.WalletObservedAt != nil {
			updater = updater.SetWalletObservedAt(*update.WalletObservedAt)
		} else {
			updater = updater.ClearWalletObservedAt()
		}
	}
	affected, err := updater.Save(ctx)
	if err != nil {
		return false, translateUpstreamConnectionPersistenceError(err)
	}
	if affected == 0 {
		return false, nil
	}
	if !update.GroupsObserved {
		return true, nil
	}
	if _, err := client.UpstreamGroup.Delete().Where(upstreamgroup.ConnectionIDEQ(id)).Exec(ctx); err != nil {
		return false, fmt.Errorf("replace upstream groups: %w", err)
	}
	if len(update.Groups) == 0 {
		return true, nil
	}
	builders := make([]*dbent.UpstreamGroupCreate, 0, len(update.Groups))
	for _, group := range update.Groups {
		builder := client.UpstreamGroup.Create().
			SetConnectionID(id).
			SetRemoteID(group.RemoteID).
			SetName(group.Name).
			SetSource(group.Source).
			SetConfidence(group.Confidence).
			SetMetadata(nonNilJSONMap(group.Metadata)).
			SetNillableRateMultiplier(group.RateMultiplier).
			SetNillableObservedAt(group.ObservedAt).
			SetNillableFreshUntil(group.FreshUntil)
		builders = append(builders, builder)
	}
	if _, err := client.UpstreamGroup.CreateBulk(builders...).Save(ctx); err != nil {
		return false, fmt.Errorf("create upstream group snapshot: %w", err)
	}
	return true, nil
}

func (r *upstreamConnectionRepository) RecordProbeFailure(
	ctx context.Context,
	id, expectedVersion int64,
	failure service.UpstreamConnectionProbeFailure,
) (bool, error) {
	client := clientFromContext(ctx, r.client)
	updater := client.UpstreamConnection.Update().
		Where(upstreamconnection.IDEQ(id), upstreamconnection.VersionEQ(expectedVersion)).
		SetStatus(failure.Status).
		SetLastError(failure.LastError).
		SetSyncFailures(failure.SyncFailures).
		SetVersion(failure.Version)
	if failure.NextSyncAt != nil {
		updater = updater.SetNextSyncAt(*failure.NextSyncAt)
	} else {
		updater = updater.ClearNextSyncAt()
	}
	affected, err := updater.Save(ctx)
	if err != nil {
		return false, translateUpstreamConnectionPersistenceError(err)
	}
	return affected == 1, nil
}

func (r *upstreamConnectionRepository) ListDueConnections(ctx context.Context, now time.Time, limit int) ([]*service.UpstreamConnection, error) {
	if limit < 1 {
		return []*service.UpstreamConnection{}, nil
	}
	client := clientFromContext(ctx, r.client)
	rows, err := client.UpstreamConnection.Query().
		Where(
			upstreamconnection.SyncEnabledEQ(true),
			upstreamconnection.Or(
				upstreamconnection.NextSyncAtIsNil(),
				upstreamconnection.NextSyncAtLTE(now),
			),
		).
		Order(dbent.Asc(upstreamconnection.FieldNextSyncAt), dbent.Asc(upstreamconnection.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list due upstream connections: %w", err)
	}
	items := make([]*service.UpstreamConnection, 0, len(rows))
	for _, row := range rows {
		items = append(items, entUpstreamConnectionToService(row, false))
	}
	return items, nil
}

func (r *upstreamConnectionRepository) ListDueAccountBindings(ctx context.Context, connectionID int64, now time.Time, limit int) ([]service.UpstreamAccountBinding, error) {
	if limit < 1 {
		return []service.UpstreamAccountBinding{}, nil
	}
	client := clientFromContext(ctx, r.client)
	rows, err := client.UpstreamAccountBinding.Query().
		Where(
			upstreamaccountbinding.ConnectionIDEQ(connectionID),
			upstreamaccountbinding.Or(
				upstreamaccountbinding.NextSyncAtIsNil(),
				upstreamaccountbinding.NextSyncAtLTE(now),
			),
		).
		Order(dbent.Asc(upstreamaccountbinding.FieldNextSyncAt), dbent.Asc(upstreamaccountbinding.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list due upstream account bindings: %w", err)
	}
	items := make([]service.UpstreamAccountBinding, 0, len(rows))
	for _, row := range rows {
		items = append(items, entUpstreamBindingToService(row))
	}
	return items, nil
}

func (r *upstreamConnectionRepository) UpsertAccountBindingIfCurrent(
	ctx context.Context,
	binding *service.UpstreamAccountBinding,
	expectedConnectionVersion int64,
) (bool, error) {
	if existingTx := dbent.TxFromContext(ctx); existingTx != nil {
		return upsertAccountBindingIfCurrentWithClient(ctx, existingTx.Client(), binding, expectedConnectionVersion)
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return false, fmt.Errorf("begin upstream account binding update: %w", err)
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	applied, err := upsertAccountBindingIfCurrentWithClient(txCtx, tx.Client(), binding, expectedConnectionVersion)
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit upstream account binding update: %w", err)
	}
	return applied, nil
}

func upsertAccountBindingIfCurrentWithClient(
	ctx context.Context,
	client *dbent.Client,
	binding *service.UpstreamAccountBinding,
	expectedConnectionVersion int64,
) (bool, error) {
	_, err := client.UpstreamConnection.Query().
		Where(
			upstreamconnection.IDEQ(binding.ConnectionID),
			upstreamconnection.VersionEQ(expectedConnectionVersion),
		).
		ForUpdate().
		Only(ctx)
	if dbent.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, translateUpstreamConnectionPersistenceError(err)
	}
	builder := client.UpstreamAccountBinding.Create().
		SetAccountID(binding.AccountID).
		SetConnectionID(binding.ConnectionID).
		SetKeyFingerprint(binding.KeyFingerprint).
		SetRemoteTokenID(binding.RemoteTokenID).
		SetRemoteTokenName(binding.RemoteTokenName).
		SetResolutionKind(binding.ResolutionKind).
		SetRemoteGroupID(binding.RemoteGroupID).
		SetRemoteGroupName(binding.RemoteGroupName).
		SetFallbackGroups(nonNilStrings(binding.FallbackGroups)).
		SetConfidence(binding.Confidence).
		SetSource(binding.Source).
		SetApplyPolicy(binding.ApplyPolicy).
		SetStatus(binding.Status).
		SetSyncFailures(binding.SyncFailures).
		SetLastError(binding.LastError).
		SetResolutionDetails(nonNilJSONMap(binding.ResolutionDetails)).
		SetNillableObservedMultiplier(binding.ObservedMultiplier).
		SetNillableObservedAt(binding.ObservedAt).
		SetNillableFreshUntil(binding.FreshUntil).
		SetNillableNextSyncAt(binding.NextSyncAt)
	upsert := builder.OnConflictColumns(upstreamaccountbinding.FieldAccountID).UpdateNewValues()
	if binding.ObservedMultiplier == nil {
		upsert = upsert.ClearObservedMultiplier()
	}
	if binding.ObservedAt == nil {
		upsert = upsert.ClearObservedAt()
	}
	if binding.FreshUntil == nil {
		upsert = upsert.ClearFreshUntil()
	}
	if binding.NextSyncAt == nil {
		upsert = upsert.ClearNextSyncAt()
	}
	id, err := upsert.ID(ctx)
	if err != nil {
		return false, translateUpstreamConnectionPersistenceError(err)
	}
	row, err := client.UpstreamAccountBinding.Get(ctx, id)
	if err != nil {
		return false, translateUpstreamConnectionPersistenceError(err)
	}
	converted := entUpstreamBindingToService(row)
	*binding = converted
	return true, nil
}

func (r *upstreamConnectionRepository) UpdateAccountBindingIfCurrent(
	ctx context.Context,
	binding *service.UpstreamAccountBinding,
	expectedConnectionID, expectedConnectionVersion int64,
) (bool, error) {
	client := clientFromContext(ctx, r.client)
	updater := client.UpstreamAccountBinding.Update().
		Where(
			upstreamaccountbinding.IDEQ(binding.ID),
			upstreamaccountbinding.AccountIDEQ(binding.AccountID),
			upstreamaccountbinding.ConnectionIDEQ(expectedConnectionID),
			upstreamaccountbinding.HasConnectionWith(upstreamconnection.VersionEQ(expectedConnectionVersion)),
		).
		SetConnectionID(binding.ConnectionID).
		SetKeyFingerprint(binding.KeyFingerprint).
		SetRemoteTokenID(binding.RemoteTokenID).
		SetRemoteTokenName(binding.RemoteTokenName).
		SetResolutionKind(binding.ResolutionKind).
		SetRemoteGroupID(binding.RemoteGroupID).
		SetRemoteGroupName(binding.RemoteGroupName).
		SetFallbackGroups(nonNilStrings(binding.FallbackGroups)).
		SetConfidence(binding.Confidence).
		SetSource(binding.Source).
		SetApplyPolicy(binding.ApplyPolicy).
		SetStatus(binding.Status).
		SetSyncFailures(binding.SyncFailures).
		SetLastError(binding.LastError).
		SetResolutionDetails(nonNilJSONMap(binding.ResolutionDetails))
	if binding.ObservedMultiplier != nil {
		updater = updater.SetObservedMultiplier(*binding.ObservedMultiplier)
	} else {
		updater = updater.ClearObservedMultiplier()
	}
	if binding.ObservedAt != nil {
		updater = updater.SetObservedAt(*binding.ObservedAt)
	} else {
		updater = updater.ClearObservedAt()
	}
	if binding.FreshUntil != nil {
		updater = updater.SetFreshUntil(*binding.FreshUntil)
	} else {
		updater = updater.ClearFreshUntil()
	}
	if binding.NextSyncAt != nil {
		updater = updater.SetNextSyncAt(*binding.NextSyncAt)
	} else {
		updater = updater.ClearNextSyncAt()
	}
	affected, err := updater.Save(ctx)
	if err != nil {
		return false, translateUpstreamConnectionPersistenceError(err)
	}
	return affected == 1, nil
}

func (r *upstreamConnectionRepository) GetAccountBinding(ctx context.Context, accountID int64) (*service.UpstreamAccountBinding, error) {
	client := clientFromContext(ctx, r.client)
	row, err := client.UpstreamAccountBinding.Query().
		Where(upstreamaccountbinding.AccountIDEQ(accountID)).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return nil, service.ErrUpstreamAccountBindingNotFound.WithCause(err)
	}
	if err != nil {
		return nil, err
	}
	converted := entUpstreamBindingToService(row)
	return &converted, nil
}

func (r *upstreamConnectionRepository) DeleteAccountBinding(ctx context.Context, connectionID, accountID int64) error {
	client := clientFromContext(ctx, r.client)
	deleted, err := client.UpstreamAccountBinding.Delete().Where(
		upstreamaccountbinding.ConnectionIDEQ(connectionID),
		upstreamaccountbinding.AccountIDEQ(accountID),
	).Exec(ctx)
	if err != nil {
		return err
	}
	if deleted == 0 {
		return service.ErrUpstreamAccountBindingNotFound
	}
	return nil
}

func entUpstreamConnectionToService(row *dbent.UpstreamConnection, includeDetails bool) *service.UpstreamConnection {
	if row == nil {
		return nil
	}
	connection := &service.UpstreamConnection{}
	applyEntUpstreamConnection(connection, row, includeDetails)
	return connection
}

func applyEntUpstreamConnection(connection *service.UpstreamConnection, row *dbent.UpstreamConnection, includeDetails bool) {
	connection.ID = row.ID
	connection.Name = row.Name
	connection.Provider = row.Provider
	connection.AuthMode = row.AuthMode
	connection.ManagementBaseURL = row.ManagementBaseURL
	connection.ForwardingBaseURL = row.ForwardingBaseURL
	connection.CredentialEncrypted = row.CredentialEncrypted
	connection.CredentialFingerprint = row.CredentialFingerprint
	connection.LegacyMigrationKey = ""
	if row.LegacyMigrationKey != nil {
		connection.LegacyMigrationKey = *row.LegacyMigrationKey
	}
	connection.CredentialHint = row.CredentialHint
	connection.RemoteUserID = row.RemoteUserID
	connection.ProxyID = cloneInt64(row.ProxyID)
	connection.Capabilities = nonNilJSONMap(row.Capabilities)
	connection.Status = row.Status
	connection.LastError = row.LastError
	connection.SyncEnabled = row.SyncEnabled
	connection.SyncIntervalSeconds = row.SyncIntervalSeconds
	connection.SyncFailures = row.SyncFailures
	connection.Version = row.Version
	connection.WalletAmount = cloneFloat64(row.WalletAmount)
	connection.WalletCurrency = row.WalletCurrency
	connection.WalletUSD = cloneFloat64(row.WalletUsd)
	connection.WalletUnlimited = row.WalletUnlimited
	connection.WalletSource = row.WalletSource
	connection.WalletReliability = row.WalletReliability
	connection.WalletRaw = nonNilJSONMap(row.WalletRaw)
	connection.WalletObservedAt = cloneTime(row.WalletObservedAt)
	connection.LastDiscoveredAt = cloneTime(row.LastDiscoveredAt)
	connection.LastSyncedAt = cloneTime(row.LastSyncedAt)
	connection.NextSyncAt = cloneTime(row.NextSyncAt)
	connection.CreatedAt = row.CreatedAt
	connection.UpdatedAt = row.UpdatedAt
	connection.GroupCount = len(row.Edges.Groups)
	connection.BindingCount = len(row.Edges.AccountBindings)
	connection.Groups = []service.UpstreamGroup{}
	connection.Bindings = []service.UpstreamAccountBinding{}
	if !includeDetails {
		return
	}
	for _, group := range row.Edges.Groups {
		connection.Groups = append(connection.Groups, entUpstreamGroupToService(group))
	}
	for _, binding := range row.Edges.AccountBindings {
		connection.Bindings = append(connection.Bindings, entUpstreamBindingToService(binding))
	}
}

func entUpstreamGroupToService(row *dbent.UpstreamGroup) service.UpstreamGroup {
	return service.UpstreamGroup{
		ID: row.ID, ConnectionID: row.ConnectionID, RemoteID: row.RemoteID, Name: row.Name,
		RateMultiplier: cloneFloat64(row.RateMultiplier), Source: row.Source, Confidence: row.Confidence,
		Metadata: nonNilJSONMap(row.Metadata), ObservedAt: cloneTime(row.ObservedAt), FreshUntil: cloneTime(row.FreshUntil),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func entUpstreamBindingToService(row *dbent.UpstreamAccountBinding) service.UpstreamAccountBinding {
	return service.UpstreamAccountBinding{
		ID: row.ID, AccountID: row.AccountID, ConnectionID: row.ConnectionID, KeyFingerprint: row.KeyFingerprint,
		RemoteTokenID: row.RemoteTokenID, RemoteTokenName: row.RemoteTokenName, ResolutionKind: row.ResolutionKind,
		RemoteGroupID: row.RemoteGroupID, RemoteGroupName: row.RemoteGroupName,
		FallbackGroups: append([]string{}, row.FallbackGroups...), ObservedMultiplier: cloneFloat64(row.ObservedMultiplier),
		Confidence: row.Confidence, Source: row.Source, ApplyPolicy: row.ApplyPolicy, Status: row.Status,
		SyncFailures: row.SyncFailures, LastError: row.LastError, ResolutionDetails: nonNilJSONMap(row.ResolutionDetails),
		ObservedAt: cloneTime(row.ObservedAt), FreshUntil: cloneTime(row.FreshUntil), NextSyncAt: cloneTime(row.NextSyncAt),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func translateUpstreamConnectionPersistenceError(err error) error {
	if err == nil {
		return nil
	}
	if dbent.IsNotFound(err) {
		return service.ErrUpstreamConnectionNotFound.WithCause(err)
	}
	var postgresError *pq.Error
	if errors.As(err, &postgresError) && postgresError.Code == "23503" {
		return service.ErrUpstreamConnectionInvalidReference.WithCause(err)
	}
	return err
}

func nonNilJSONMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func nonNilStrings(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
