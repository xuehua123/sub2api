package service

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	settingKeyBackupS3Config = "backup_s3_config"
	settingKeyBackupSchedule = "backup_schedule"
	settingKeyBackupRecords  = "backup_records"

	backupObjectCleanupTimeout = 2 * time.Minute
	backupRecoveryInterval     = time.Minute
	backupRecoverySweepTimeout = backupObjectCleanupTimeout + 30*time.Second
	defaultBackupRetainDays    = 14
	backupOperationTimeout     = 30 * time.Minute
	backupOperationLockTTL     = 45 * time.Minute
	backupStaleGrace           = 15 * time.Minute
	backupRecordsLockTTL       = 30 * time.Second
	backupOperationLockKey     = "backup:operation"
	backupRecordsLockKey       = "backup:records"
	backupRecoveryLockKey      = "backup:recovery"
)

var (
	ErrBackupS3NotConfigured             = infraerrors.BadRequest("BACKUP_S3_NOT_CONFIGURED", "backup S3 storage is not configured")
	ErrBackupNotFound                    = infraerrors.NotFound("BACKUP_NOT_FOUND", "backup record not found")
	ErrBackupInProgress                  = infraerrors.Conflict("BACKUP_IN_PROGRESS", "a backup is already in progress")
	ErrRestoreInProgress                 = infraerrors.Conflict("RESTORE_IN_PROGRESS", "a restore is already in progress")
	ErrRestoreRequiresOfflineMaintenance = infraerrors.Conflict(
		"RESTORE_REQUIRES_OFFLINE_MAINTENANCE",
		"database restore is disabled in the online server; drain traffic, stop every application slot and worker, then use the reviewed offline restore procedure",
	)
	ErrBackupCoordinationUnavailable = infraerrors.ServiceUnavailable(
		"BACKUP_COORDINATION_UNAVAILABLE",
		"backup coordination is temporarily unavailable",
	)
	ErrBackupS3StorageInUse = infraerrors.Conflict(
		"BACKUP_S3_STORAGE_IN_USE",
		"cannot change the backup storage location while backup records still reference objects there; delete those backups first",
	)
	ErrBackupS3SecretRequired = infraerrors.BadRequest(
		"BACKUP_S3_SECRET_REQUIRED",
		"secret_access_key is required when changing access_key_id",
	)
	ErrBackupRecordsCorrupt  = infraerrors.InternalServer("BACKUP_RECORDS_CORRUPT", "backup records data is corrupted")
	ErrBackupS3ConfigCorrupt = infraerrors.InternalServer("BACKUP_S3_CONFIG_CORRUPT", "backup S3 config data is corrupted")

	// ErrSecretEncryptionKeyNotConfigured is returned when an S3 SecretAccessKey
	// would be encrypted with an auto-generated (ephemeral) key. That key is
	// regenerated on every process start, so the persisted ciphertext becomes
	// undecryptable after a restart/upgrade ("cipher: message authentication
	// failed"), silently breaking S3 backup/image storage (#4524). Mirrors the
	// existing guards for payments (payment.ProvideEncryptionKey) and TOTP
	// enablement, which likewise refuse to depend on an auto-generated key.
	ErrSecretEncryptionKeyNotConfigured = infraerrors.BadRequest(
		"SECRET_ENCRYPTION_KEY_NOT_CONFIGURED",
		"cannot store the S3 secret access key: no fixed secret encryption key is configured, so the auto-generated key would change on every restart and make the stored secret undecryptable after a restart or upgrade. Set a fixed TOTP_ENCRYPTION_KEY (e.g. generate one with `openssl rand -hex 32`) and try again",
	)
)

// ─── 接口定义 ───

// DBDumper abstracts database dump/restore operations
type DBDumper interface {
	Dump(ctx context.Context) (io.ReadCloser, error)
	Restore(ctx context.Context, data io.Reader) error
}

// BackupObjectStore abstracts object storage for backup files
type BackupObjectStore interface {
	Upload(ctx context.Context, key string, body io.Reader, contentType string) (sizeBytes int64, err error)
	UploadFile(ctx context.Context, key string, filePath string, contentType string) (sizeBytes int64, err error)
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	PresignURL(ctx context.Context, key string, expiry time.Duration) (string, error)
	HeadBucket(ctx context.Context) error
}

// BackupObjectStoreFactory creates an object store from S3 config
type BackupObjectStoreFactory func(ctx context.Context, cfg *BackupS3Config) (BackupObjectStore, error)

// ─── 数据模型 ───

// BackupS3Config S3 兼容存储配置（支持 Cloudflare R2）
type BackupS3Config struct {
	Endpoint        string `json:"endpoint"` // e.g. https://<account_id>.r2.cloudflarestorage.com
	Region          string `json:"region"`   // R2 用 "auto"
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key,omitempty"` //nolint:revive // field name follows AWS convention
	Prefix          string `json:"prefix"`                      // S3 key 前缀，如 "backups/"
	ForcePathStyle  bool   `json:"force_path_style"`
}

// IsConfigured 检查必要字段是否已配置
func (c *BackupS3Config) IsConfigured() bool {
	return c.Bucket != "" && c.AccessKeyID != "" && c.SecretAccessKey != ""
}

// BackupScheduleConfig 定时备份配置
type BackupScheduleConfig struct {
	Enabled     bool   `json:"enabled"`
	CronExpr    string `json:"cron_expr"`    // cron 表达式，如 "0 2 * * *" 每天凌晨2点
	RetainDays  int    `json:"retain_days"`  // 备份文件过期天数，默认14，0=不自动清理
	RetainCount int    `json:"retain_count"` // 最多保留份数，0=不限制
}

// BackupRecord 备份记录
type BackupRecord struct {
	ID               string       `json:"id"`
	Status           string       `json:"status"`      // pending, running, completed, failed
	BackupType       string       `json:"backup_type"` // postgres
	FileName         string       `json:"file_name"`
	S3Key            string       `json:"s3_key"`
	Parts            []BackupPart `json:"parts,omitempty"`
	SizeBytes        int64        `json:"size_bytes"`
	TriggeredBy      string       `json:"triggered_by"` // manual, scheduled
	ErrorMsg         string       `json:"error_message,omitempty"`
	StartedAt        string       `json:"started_at"`
	FinishedAt       string       `json:"finished_at,omitempty"`
	ExpiresAt        string       `json:"expires_at,omitempty"`     // 过期时间
	Progress         string       `json:"progress,omitempty"`       // "dumping", "uploading", ""
	RestoreStatus    string       `json:"restore_status,omitempty"` // "", "running", "completed", "failed"
	RestoreError     string       `json:"restore_error,omitempty"`
	RestoredAt       string       `json:"restored_at,omitempty"`
	RestoreStartedAt string       `json:"restore_started_at,omitempty"`
}

// BackupDownloadPart 描述一个可下载的备份分卷。
type BackupDownloadPart struct {
	Index     int    `json:"index"`
	SizeBytes int64  `json:"size_bytes"`
	URL       string `json:"url"`
}

// BackupDownloadResponse 是单文件和分卷下载响应的兼容表示。
type BackupDownloadResponse struct {
	URL   string               `json:"url,omitempty"`
	Parts []BackupDownloadPart `json:"parts,omitempty"`
}

// BackupService 数据库备份恢复服务
type BackupService struct {
	settingRepo SettingRepository
	dbCfg       *config.DatabaseConfig
	encryptor   SecretEncryptor
	// encryptionKeyConfigured mirrors cfg.Totp.EncryptionKeyConfigured: false
	// means the secret encryption key was auto-generated and does not survive a
	// restart. Durable-secret writers must refuse to persist new secrets in that
	// mode (#4524).
	encryptionKeyConfigured bool
	storeFactory            BackupObjectStoreFactory
	dumper                  DBDumper

	opMu      sync.Mutex // 保护 backingUp 标志
	backingUp bool

	storeMu sync.Mutex // 保护 store/s3Cfg 缓存
	store   BackupObjectStore
	s3Cfg   *BackupS3Config

	recordsMu sync.Mutex // 保护 records 的 load/save 操作

	lockCache  LeaderLockCache
	db         *sql.DB
	instanceID string

	cronMu          sync.Mutex
	cronSched       *cron.Cron
	cronEntryID     cron.EntryID
	recoveryEntryID cron.EntryID
	recoveryCtx     context.Context
	recoveryCancel  context.CancelFunc
	recoveryStartMu sync.Mutex
	recoveryWG      sync.WaitGroup

	wg            sync.WaitGroup     // 追踪活跃的备份/恢复 goroutine
	shuttingDown  atomic.Bool        // 阻止新备份启动
	bgCtx         context.Context    // 所有后台操作的 parent context
	bgCancel      context.CancelFunc // 取消所有活跃后台操作
	partSizeBytes int64              // 分卷阈值；生产使用 4 GiB，测试可注入更小值
}

func NewBackupService(
	settingRepo SettingRepository,
	cfg *config.Config,
	encryptor SecretEncryptor,
	storeFactory BackupObjectStoreFactory,
	dumper DBDumper,
) *BackupService {
	return newBackupServiceWithCoordination(settingRepo, cfg, encryptor, storeFactory, dumper, nil, nil)
}

func newBackupServiceWithCoordination(
	settingRepo SettingRepository,
	cfg *config.Config,
	encryptor SecretEncryptor,
	storeFactory BackupObjectStoreFactory,
	dumper DBDumper,
	lockCache LeaderLockCache,
	db *sql.DB,
) *BackupService {
	bgCtx, bgCancel := context.WithCancel(context.Background())
	return &BackupService{
		settingRepo:             settingRepo,
		dbCfg:                   &cfg.Database,
		encryptor:               encryptor,
		encryptionKeyConfigured: cfg.Totp.EncryptionKeyConfigured,
		storeFactory:            storeFactory,
		dumper:                  dumper,
		bgCtx:                   bgCtx,
		bgCancel:                bgCancel,
		partSizeBytes:           defaultBackupPartSizeBytes,
		lockCache:               lockCache,
		db:                      db,
		instanceID:              uuid.NewString(),
	}
}

// Start 启动定时备份调度器并清理孤立记录。
//
// Every instance starts the scheduler; distributed locks elect the worker for
// each shared operation.
func (s *BackupService) Start() {
	if s == nil {
		return
	}
	s.cronSched = cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))
	recoveryCtx, recoveryCancel := context.WithCancel(s.bgCtx)
	s.recoveryCtx = recoveryCtx
	s.recoveryCancel = recoveryCancel
	recoveryEntryID, err := s.cronSched.AddFunc("@every "+backupRecoveryInterval.String(), s.runRecoverySweep)
	if err != nil {
		logger.LegacyPrintf("service.backup", "[Backup] 注册周期恢复扫描失败: %v", err)
	} else {
		s.recoveryEntryID = recoveryEntryID
	}
	s.cronSched.Start()

	// 立即扫描一次；随后周期任务会处理启动时尚未达到 stale 阈值的记录，
	// 也会在 Redis 短暂不可用后自动重试。
	s.runRecoverySweep()

	// 加载已有的定时配置
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	schedule, err := s.GetSchedule(ctx)
	if err != nil {
		logger.LegacyPrintf("service.backup", "[Backup] 加载定时备份配置失败: %v", err)
		return
	}
	if schedule.Enabled && schedule.CronExpr != "" {
		if err := s.applyCronSchedule(schedule); err != nil {
			logger.LegacyPrintf("service.backup", "[Backup] 应用定时备份配置失败: %v", err)
		}
	}
}

// backupOperationIsStale reports whether a backup/restore timestamp is old
// enough to treat as interrupted. Empty or unparseable timestamps are stale:
// the multi-instance grace only applies to a live operation that recorded a
// valid start time. Missing RestoreStartedAt on a pre-fusion record must not
// stay "running" forever.
func backupOperationIsStale(raw string, now time.Time) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	startedAt, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return true
	}
	return now.Sub(startedAt) > backupOperationTimeout+backupStaleGrace
}

// runRecoverySweep scans for interrupted operations. The cron scheduler wraps
// it with SkipIfStillRunning, while the distributed recovery lock elects one
// instance across a blue-green deployment.
func (s *BackupService) runRecoverySweep() {
	if s == nil {
		return
	}
	s.recoveryStartMu.Lock()
	if s.shuttingDown.Load() {
		s.recoveryStartMu.Unlock()
		return
	}
	s.recoveryWG.Add(1)
	s.recoveryStartMu.Unlock()
	defer s.recoveryWG.Done()
	ctx, cancel := context.WithTimeout(s.recoveryCtx, backupRecoverySweepTimeout)
	defer cancel()
	if err := s.recoverStaleRecordsWithContext(ctx); err != nil {
		logger.LegacyPrintf("service.backup", "[Backup] recovery sweep failed: %v", err)
	}
}

// recoverStaleRecordsWithContext marks interrupted running records failed and
// cleans any uploaded objects. Lock/backend failures are returned so the
// periodic caller can report them and retry on the next tick.
func (s *BackupService) recoverStaleRecordsWithContext(ctx context.Context) error {
	if s == nil {
		return nil
	}
	lockCtx, lockCancel := context.WithTimeout(ctx, 10*time.Second)
	defer lockCancel()
	release, acquired, lockErr := s.tryAcquireLock(lockCtx, backupRecoveryLockKey, backupOperationLockTTL)
	if lockErr != nil {
		return fmt.Errorf("acquire recovery lock: %w", lockErr)
	}
	if !acquired {
		return nil
	}
	defer release()
	operationRelease, operationAcquired, operationErr := s.tryAcquireLock(ctx, backupOperationLockKey, backupOperationLockTTL)
	if operationErr != nil {
		return fmt.Errorf("acquire backup operation lock for recovery: %w", operationErr)
	}
	if !operationAcquired {
		// A live backup/restore/delete owns the operation lock. Its records may
		// legitimately be running, so defer recovery until the next sweep.
		return nil
	}
	defer operationRelease()
	loadCtx, loadCancel := context.WithTimeout(ctx, 10*time.Second)
	defer loadCancel()

	records, err := s.loadRecords(loadCtx)
	if err != nil {
		return err
	}
	now := time.Now()
	var recoveryErrs []error
	for i := range records {
		if backupOperationIsStale(records[i].StartedAt, now) && records[i].Status == "running" {
			staleRecord := cloneBackupRecord(records[i])
			updated, saveErr := s.updateRecordIf(ctx, staleRecord.ID, func(current *BackupRecord) bool {
				return current.Status == "running" && current.StartedAt == staleRecord.StartedAt
			}, func(current *BackupRecord) {
				current.Status = "failed"
				current.ErrorMsg = "interrupted by server restart"
				current.Progress = ""
				current.FinishedAt = time.Now().Format(time.RFC3339)
			})
			if saveErr != nil {
				recoveryErrs = append(recoveryErrs, saveErr)
				continue
			}
			if !updated {
				continue
			}

			if cleanupErr := s.cleanupStaleBackupObjects(ctx, &staleRecord); cleanupErr != nil {
				_, saveErr := s.updateRecordIf(ctx, staleRecord.ID, func(current *BackupRecord) bool {
					return current.Status == "failed" && current.StartedAt == staleRecord.StartedAt
				}, func(current *BackupRecord) {
					current.ErrorMsg = fmt.Sprintf("interrupted by server restart; cleanup failed, manual deletion may be required: %v", cleanupErr)
				})
				if saveErr != nil {
					recoveryErrs = append(recoveryErrs, saveErr)
				}
				recoveryErrs = append(recoveryErrs, fmt.Errorf("cleanup stale backup %s: %w", records[i].ID, cleanupErr))
				logger.LegacyPrintf("service.backup", "[Backup] failed to clean stale backup objects for %s: %v", records[i].ID, cleanupErr)
			}
			logger.LegacyPrintf("service.backup", "[Backup] recovered stale running record: %s", records[i].ID)
		}
		if backupOperationIsStale(records[i].RestoreStartedAt, now) && records[i].RestoreStatus == "running" {
			staleRestore := cloneBackupRecord(records[i])
			updated, saveErr := s.updateRecordIf(ctx, staleRestore.ID, func(current *BackupRecord) bool {
				return current.RestoreStatus == "running" && current.RestoreStartedAt == staleRestore.RestoreStartedAt
			}, func(current *BackupRecord) {
				current.RestoreStatus = "failed"
				current.RestoreError = "interrupted by server restart"
				current.RestoredAt = ""
			})
			if saveErr != nil {
				recoveryErrs = append(recoveryErrs, saveErr)
				continue
			}
			if !updated {
				continue
			}
			logger.LegacyPrintf("service.backup", "[Backup] recovered stale restoring record: %s", records[i].ID)
		}
	}
	return errors.Join(recoveryErrs...)
}

func (s *BackupService) cleanupStaleBackupObjects(parent context.Context, record *BackupRecord) error {
	if len(backupObjectKeys(record)) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, backupObjectCleanupTimeout)
	defer cancel()
	return s.deleteBackupObjects(ctx, record)
}

// Stop 停止定时备份并等待活跃操作完成
func (s *BackupService) Stop() {
	s.shuttingDown.Store(true)
	s.recoveryStartMu.Lock()
	if s.recoveryCancel != nil {
		s.recoveryCancel()
	}
	s.recoveryStartMu.Unlock()

	s.cronMu.Lock()
	if s.cronSched != nil {
		_ = s.cronSched.Stop()
	}
	s.cronMu.Unlock()
	s.recoveryWG.Wait()

	// 等待活跃备份/恢复完成（最多 5 分钟）
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		logger.LegacyPrintf("service.backup", "[Backup] all active operations finished")
	case <-time.After(5 * time.Minute):
		logger.LegacyPrintf("service.backup", "[Backup] shutdown timeout after 5min, cancelling active operations")
		if s.bgCancel != nil {
			s.bgCancel() // 取消所有后台操作
		}
		// 给 goroutine 时间响应取消并完成清理
		select {
		case <-done:
			logger.LegacyPrintf("service.backup", "[Backup] active operations cancelled and cleaned up")
		case <-time.After(10 * time.Second):
			logger.LegacyPrintf("service.backup", "[Backup] goroutine cleanup timed out")
		}
	}
}

// ─── S3 配置管理 ───

// EncryptionKeyConfigured reports whether a fixed (explicitly configured) secret
// encryption key is in use. When false the key is auto-generated on every start
// and secrets encrypted with it cannot be recovered after a restart, so callers
// that persist durable secrets must refuse to do so (#4524).
func (s *BackupService) EncryptionKeyConfigured() bool {
	return s != nil && s.encryptionKeyConfigured
}

func (s *BackupService) GetS3Config(ctx context.Context) (*BackupS3Config, error) {
	cfg, err := s.loadS3Config(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &BackupS3Config{}, nil
	}
	// 脱敏返回
	cfg.SecretAccessKey = ""
	return cfg, nil
}

func (s *BackupService) UpdateS3Config(ctx context.Context, cfg BackupS3Config) (*BackupS3Config, error) {
	release, acquired, err := s.acquireLock(ctx, backupOperationLockKey, backupOperationLockTTL)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, ErrBackupInProgress
	}
	defer release()

	oldResolved, err := s.loadS3Config(ctx)
	if err != nil {
		return nil, err
	}
	oldStored, err := s.loadStoredS3Config(ctx)
	if err != nil {
		return nil, err
	}
	runtimeCfg := cfg
	secretProvided := cfg.SecretAccessKey != ""
	if oldResolved != nil && !secretProvided && cfg.AccessKeyID != oldResolved.AccessKeyID {
		return nil, ErrBackupS3SecretRequired
	}

	// 如果没提供 secret，保留原有值
	if cfg.SecretAccessKey == "" {
		// Read the persisted representation here. loadS3Config decrypts the
		// secret, and writing that plaintext back would silently remove at-rest
		// encryption on an otherwise unrelated config update.
		if oldStored != nil {
			cfg.SecretAccessKey = oldStored.SecretAccessKey
		}
		if oldResolved != nil {
			runtimeCfg.SecretAccessKey = oldResolved.SecretAccessKey
		}
	} else {
		// 拒绝用自动生成的临时密钥加密：该密钥每次重启都会变化，落库的密文在
		// 重启/升级后无法解密（#4524）。与支付、TOTP 的处理保持一致。
		if !s.encryptionKeyConfigured {
			return nil, ErrSecretEncryptionKeyNotConfigured
		}
		// 加密 SecretAccessKey
		encrypted, err := s.encryptor.Encrypt(runtimeCfg.SecretAccessKey)
		if err != nil {
			return nil, fmt.Errorf("encrypt secret: %w", err)
		}
		cfg.SecretAccessKey = encrypted
	}

	var hasObjectRecords bool
	if err := s.withRecordsLock(ctx, func(records []BackupRecord) ([]BackupRecord, bool, error) {
		for i := range records {
			if len(backupObjectKeys(&records[i])) > 0 {
				hasObjectRecords = true
				break
			}
		}
		return records, false, nil
	}); err != nil {
		return nil, err
	}
	if hasObjectRecords {
		if !runtimeCfg.IsConfigured() {
			return nil, ErrBackupS3StorageInUse
		}
		if oldResolved == nil {
			// Existing keys have no trustworthy locator. Do not guess that an
			// arbitrary newly supplied bucket owns them; an operator must restore
			// or explicitly bind the original storage profile offline.
			return nil, ErrBackupS3StorageInUse
		}
		// Object keys already contain the prefix, so changing Prefix does not
		// orphan old backups. Credentials may also be rotated as long as they
		// still address the same location. Endpoint/bucket/signing-address
		// changes are blocked until all referenced objects are removed.
		if !backupS3StorageLocationEqual(oldResolved, &runtimeCfg) {
			return nil, ErrBackupS3StorageInUse
		}
	}
	if runtimeCfg.IsConfigured() {
		candidateStore, createErr := s.storeFactory(ctx, &runtimeCfg)
		if createErr != nil {
			return nil, fmt.Errorf("init candidate backup object store: %w", createErr)
		}
		if headErr := candidateStore.HeadBucket(ctx); headErr != nil {
			return nil, fmt.Errorf("validate candidate backup object store: %w", headErr)
		}
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal s3 config: %w", err)
	}
	if err := s.settingRepo.Set(ctx, settingKeyBackupS3Config, string(data)); err != nil {
		return nil, fmt.Errorf("save s3 config: %w", err)
	}

	// 清除缓存的 S3 客户端
	s.storeMu.Lock()
	s.store = nil
	s.s3Cfg = nil
	s.storeMu.Unlock()

	cfg.SecretAccessKey = ""
	return &cfg, nil
}

func (s *BackupService) TestS3Connection(ctx context.Context, cfg BackupS3Config) error {
	// 如果没提供 secret，用已保存的
	if cfg.SecretAccessKey == "" {
		old, err := s.loadS3Config(ctx)
		if err != nil {
			return err
		}
		if old != nil {
			cfg.SecretAccessKey = old.SecretAccessKey
		}
	}

	if cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return fmt.Errorf("incomplete S3 config: bucket, access_key_id, secret_access_key are required")
	}

	store, err := s.storeFactory(ctx, &cfg)
	if err != nil {
		return err
	}
	return store.HeadBucket(ctx)
}

// ─── 定时备份管理 ───

func (s *BackupService) GetSchedule(ctx context.Context) (*BackupScheduleConfig, error) {
	raw, err := s.settingRepo.GetValue(ctx, settingKeyBackupSchedule)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return &BackupScheduleConfig{RetainDays: defaultBackupRetainDays}, nil
		}
		return nil, fmt.Errorf("load backup schedule: %w", err)
	}
	if raw == "" {
		return &BackupScheduleConfig{RetainDays: defaultBackupRetainDays}, nil
	}
	var cfg BackupScheduleConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("decode backup schedule: %w", err)
	}
	return &cfg, nil
}

func (s *BackupService) UpdateSchedule(ctx context.Context, cfg BackupScheduleConfig) (*BackupScheduleConfig, error) {
	if cfg.Enabled && cfg.CronExpr == "" {
		return nil, infraerrors.BadRequest("INVALID_CRON", "cron expression is required when schedule is enabled")
	}
	// 验证 cron 表达式
	if cfg.CronExpr != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(cfg.CronExpr); err != nil {
			return nil, infraerrors.BadRequest("INVALID_CRON", fmt.Sprintf("invalid cron expression: %v", err))
		}
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal schedule config: %w", err)
	}
	if err := s.settingRepo.Set(ctx, settingKeyBackupSchedule, string(data)); err != nil {
		return nil, fmt.Errorf("save schedule config: %w", err)
	}

	// 应用或停止定时任务。Inactive blue-green slots still persist the
	// operator's desired schedule, but never activate a local cron job.
	if cfg.Enabled {
		if err := s.applyCronSchedule(&cfg); err != nil {
			return nil, err
		}
	} else {
		s.removeCronSchedule()
	}

	return &cfg, nil
}

func (s *BackupService) applyCronSchedule(cfg *BackupScheduleConfig) error {
	if s == nil {
		return nil
	}
	s.cronMu.Lock()
	defer s.cronMu.Unlock()

	if s.cronSched == nil {
		return fmt.Errorf("cron scheduler not initialized")
	}

	// 移除旧任务
	if s.cronEntryID != 0 {
		s.cronSched.Remove(s.cronEntryID)
		s.cronEntryID = 0
	}

	entryID, err := s.cronSched.AddFunc(cfg.CronExpr, func() {
		s.runScheduledBackup()
	})
	if err != nil {
		return infraerrors.BadRequest("INVALID_CRON", fmt.Sprintf("failed to schedule: %v", err))
	}
	s.cronEntryID = entryID
	logger.LegacyPrintf("service.backup", "[Backup] 定时备份已启用: %s", cfg.CronExpr)
	return nil
}

func (s *BackupService) removeCronSchedule() {
	s.cronMu.Lock()
	defer s.cronMu.Unlock()
	if s.cronSched != nil && s.cronEntryID != 0 {
		s.cronSched.Remove(s.cronEntryID)
		s.cronEntryID = 0
		logger.LegacyPrintf("service.backup", "[Backup] 定时备份已停用")
	}
}

func (s *BackupService) runScheduledBackup() {
	if s == nil {
		return
	}
	s.wg.Add(1)
	defer s.wg.Done()

	ctx, cancel := context.WithTimeout(s.bgCtx, 30*time.Minute)
	defer cancel()
	release, acquired, lockErr := s.tryAcquireLock(ctx, backupOperationLockKey, backupOperationLockTTL)
	if lockErr != nil {
		logger.LegacyPrintf("service.backup", "[Backup] 定时备份协调失败: %v", lockErr)
		return
	}
	if !acquired {
		logger.LegacyPrintf("service.backup", "[Backup] 定时备份跳过: 另一实例正在执行")
		return
	}
	defer release()

	// 读取定时备份配置中的过期天数
	schedule, err := s.GetSchedule(ctx)
	if err != nil {
		logger.LegacyPrintf("service.backup", "[Backup] 加载定时备份配置失败: %v", err)
		return
	}
	expireDays := defaultBackupRetainDays // 未配置时默认14天过期
	if schedule != nil {
		// retain_days=0 explicitly means never expire; do not treat it as
		// an omitted value and silently reintroduce the 14-day default.
		expireDays = schedule.RetainDays
	}

	logger.LegacyPrintf("service.backup", "[Backup] 开始执行定时备份, 过期天数: %d", expireDays)
	record, err := s.createBackupUnlocked(ctx, "scheduled", expireDays)
	if err != nil {
		if errors.Is(err, ErrBackupInProgress) {
			logger.LegacyPrintf("service.backup", "[Backup] 定时备份跳过: 已有备份正在进行中")
		} else {
			logger.LegacyPrintf("service.backup", "[Backup] 定时备份失败: %v", err)
		}
		return
	}
	logger.LegacyPrintf("service.backup", "[Backup] 定时备份完成: id=%s size=%d", record.ID, record.SizeBytes)

	// 清理过期备份（复用已加载的 schedule）
	if schedule == nil {
		return
	}
	if err := s.cleanupOldBackupsUnlocked(ctx, schedule); err != nil {
		logger.LegacyPrintf("service.backup", "[Backup] 清理过期备份失败: %v", err)
	}
}

// ─── 备份/恢复核心 ───

// CreateBackup 创建全量数据库备份并上传到 S3。
// expireDays: 备份过期天数，0=永不过期，默认14天
func (s *BackupService) CreateBackup(ctx context.Context, triggeredBy string, expireDays int) (*BackupRecord, error) {
	release, acquired, err := s.acquireLock(ctx, backupOperationLockKey, backupOperationLockTTL)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, ErrBackupInProgress
	}
	defer release()
	return s.createBackupUnlocked(ctx, triggeredBy, expireDays)
}

func (s *BackupService) createBackupUnlocked(ctx context.Context, triggeredBy string, expireDays int) (*BackupRecord, error) {
	if s.shuttingDown.Load() {
		return nil, infraerrors.ServiceUnavailable("SERVER_SHUTTING_DOWN", "server is shutting down")
	}

	s.opMu.Lock()
	if s.backingUp {
		s.opMu.Unlock()
		return nil, ErrBackupInProgress
	}
	s.backingUp = true
	s.opMu.Unlock()
	defer func() {
		s.opMu.Lock()
		s.backingUp = false
		s.opMu.Unlock()
	}()

	s3Cfg, err := s.loadS3Config(ctx)
	if err != nil {
		return nil, err
	}
	if s3Cfg == nil || !s3Cfg.IsConfigured() {
		return nil, ErrBackupS3NotConfigured
	}

	objectStore, err := s.getOrCreateStore(ctx, s3Cfg)
	if err != nil {
		return nil, fmt.Errorf("init object store: %w", err)
	}

	now := time.Now()
	backupID := uuid.New().String()[:8]
	fileName := fmt.Sprintf("%s_%s.sql.gz", s.dbCfg.DBName, now.Format("20060102_150405"))
	s3Key := s.buildS3Key(s3Cfg, fileName)

	var expiresAt string
	if expireDays > 0 {
		expiresAt = now.AddDate(0, 0, expireDays).Format(time.RFC3339)
	}

	record := &BackupRecord{
		ID:          backupID,
		Status:      "running",
		BackupType:  "postgres",
		FileName:    fileName,
		S3Key:       s3Key,
		TriggeredBy: triggeredBy,
		StartedAt:   now.Format(time.RFC3339),
		ExpiresAt:   expiresAt,
	}

	archivePath, sizeBytes, err := s.createCompressedBackupFile(ctx)
	if err != nil {
		record.Status = "failed"
		record.ErrorMsg = err.Error()
		record.FinishedAt = time.Now().Format(time.RFC3339)
		_ = s.saveRecord(ctx, record)
		return record, err
	}
	defer func() { _ = cleanupBackupFiles(archivePath) }()
	record.SizeBytes = sizeBytes
	if err := s.saveRecord(ctx, record); err != nil {
		return nil, fmt.Errorf("save initial record: %w", err)
	}
	if err := s.uploadBackupArchive(ctx, record, objectStore, s3Cfg, archivePath); err != nil {
		record.Status = "failed"
		record.ErrorMsg = err.Error()
		record.FinishedAt = time.Now().Format(time.RFC3339)
		_ = s.saveRecord(ctx, record)
		return record, err
	}

	record.Status = "completed"
	record.FinishedAt = time.Now().Format(time.RFC3339)
	if err := s.saveRecord(ctx, record); err != nil {
		// A backup is not safe to use for retention until its completed state is
		// durable. Returning an error keeps the scheduler from deleting an older
		// completed backup when the new object's metadata is still only running.
		return record, fmt.Errorf("save completed backup record: %w", err)
	}

	return record, nil
}

// StartBackup 异步创建备份，立即返回 running 状态的记录
func (s *BackupService) StartBackup(ctx context.Context, triggeredBy string, expireDays int) (*BackupRecord, error) {
	if s.shuttingDown.Load() {
		return nil, infraerrors.ServiceUnavailable("SERVER_SHUTTING_DOWN", "server is shutting down")
	}

	s.opMu.Lock()
	if s.backingUp {
		s.opMu.Unlock()
		return nil, ErrBackupInProgress
	}
	s.backingUp = true
	s.opMu.Unlock()
	release, acquired, lockErr := s.tryAcquireLock(ctx, backupOperationLockKey, backupOperationLockTTL)
	if lockErr != nil {
		s.opMu.Lock()
		s.backingUp = false
		s.opMu.Unlock()
		return nil, lockErr
	}
	if !acquired {
		s.opMu.Lock()
		s.backingUp = false
		s.opMu.Unlock()
		return nil, ErrBackupInProgress
	}

	// 初始化阶段出错时自动重置标志
	launched := false
	defer func() {
		if !launched {
			release()
			s.opMu.Lock()
			s.backingUp = false
			s.opMu.Unlock()
		}
	}()

	// 在返回前加载 S3 配置和创建 store，避免 goroutine 中配置被修改
	s3Cfg, err := s.loadS3Config(ctx)
	if err != nil {
		return nil, err
	}
	if s3Cfg == nil || !s3Cfg.IsConfigured() {
		return nil, ErrBackupS3NotConfigured
	}

	objectStore, err := s.getOrCreateStore(ctx, s3Cfg)
	if err != nil {
		return nil, fmt.Errorf("init object store: %w", err)
	}

	now := time.Now()
	backupID := uuid.New().String()[:8]
	fileName := fmt.Sprintf("%s_%s.sql.gz", s.dbCfg.DBName, now.Format("20060102_150405"))
	s3Key := s.buildS3Key(s3Cfg, fileName)

	var expiresAt string
	if expireDays > 0 {
		expiresAt = now.AddDate(0, 0, expireDays).Format(time.RFC3339)
	}

	record := &BackupRecord{
		ID:          backupID,
		Status:      "running",
		BackupType:  "postgres",
		FileName:    fileName,
		S3Key:       s3Key,
		TriggeredBy: triggeredBy,
		StartedAt:   now.Format(time.RFC3339),
		ExpiresAt:   expiresAt,
		Progress:    "pending",
	}

	if err := s.saveRecord(ctx, record); err != nil {
		return nil, fmt.Errorf("save initial record: %w", err)
	}

	launched = true
	// 在启动 goroutine 前完成拷贝，避免数据竞争
	result := *record

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer release()
		defer func() {
			s.opMu.Lock()
			s.backingUp = false
			s.opMu.Unlock()
		}()
		defer func() {
			if r := recover(); r != nil {
				logger.LegacyPrintf("service.backup", "[Backup] panic recovered: %v", r)
				record.Status = "failed"
				record.ErrorMsg = fmt.Sprintf("internal panic: %v", r)
				record.Progress = ""
				record.FinishedAt = time.Now().Format(time.RFC3339)
				_ = s.saveRecord(context.Background(), record)
			}
		}()
		s.executeBackup(record, objectStore, s3Cfg)
	}()

	return &result, nil
}

// executeBackup 后台执行备份（独立于 HTTP context）
func (s *BackupService) executeBackup(record *BackupRecord, objectStore BackupObjectStore, s3Cfg *BackupS3Config) {
	ctx, cancel := context.WithTimeout(s.bgCtx, 30*time.Minute)
	defer cancel()

	// 阶段1: pg_dump -> gzip 临时文件
	record.Progress = "dumping"
	_ = s.saveRecord(ctx, record)
	archivePath, sizeBytes, err := s.createCompressedBackupFile(ctx)
	if err != nil {
		record.Status = "failed"
		record.ErrorMsg = err.Error()
		record.Progress = ""
		record.FinishedAt = time.Now().Format(time.RFC3339)
		_ = s.saveRecord(context.Background(), record)
		return
	}
	defer func() { _ = cleanupBackupFiles(archivePath) }()
	record.SizeBytes = sizeBytes

	// 阶段2: 单对象或分卷上传
	record.Progress = "uploading"
	_ = s.saveRecord(ctx, record)
	if err := s.uploadBackupArchive(ctx, record, objectStore, s3Cfg, archivePath); err != nil {
		record.Status = "failed"
		record.ErrorMsg = err.Error()
		record.Progress = ""
		record.FinishedAt = time.Now().Format(time.RFC3339)
		_ = s.saveRecord(context.Background(), record)
		return
	}

	record.SizeBytes = sizeBytes
	record.Status = "completed"
	record.Progress = ""
	record.FinishedAt = time.Now().Format(time.RFC3339)
	if err := s.saveRecord(context.Background(), record); err != nil {
		logger.LegacyPrintf("service.backup", "[Backup] 保存备份记录失败: %v", err)
	}
}

func (s *BackupService) createCompressedBackupFile(ctx context.Context) (string, int64, error) {
	dumpReader, err := s.dumper.Dump(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("pg_dump: %w", err)
	}
	archive, err := os.CreateTemp("", "sub2api-backup-*.sql.gz")
	if err != nil {
		_ = dumpReader.Close()
		return "", 0, fmt.Errorf("create backup archive: %w", err)
	}
	archivePath := archive.Name()

	gzWriter := gzip.NewWriter(archive)
	_, copyErr := io.Copy(gzWriter, dumpReader)
	if closeErr := gzWriter.Close(); copyErr == nil && closeErr != nil {
		copyErr = closeErr
	}
	if closeErr := dumpReader.Close(); copyErr == nil && closeErr != nil {
		copyErr = closeErr
	}
	if closeErr := archive.Close(); copyErr == nil && closeErr != nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		_ = cleanupBackupFiles(archivePath)
		return "", 0, fmt.Errorf("gzip/dump failed: %w", copyErr)
	}

	info, err := os.Stat(archivePath)
	if err != nil {
		_ = cleanupBackupFiles(archivePath)
		return "", 0, fmt.Errorf("stat backup archive: %w", err)
	}
	return archivePath, info.Size(), nil
}

func (s *BackupService) uploadBackupArchive(ctx context.Context, record *BackupRecord, objectStore BackupObjectStore, cfg *BackupS3Config, archivePath string) error {
	info, err := os.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("stat backup archive: %w", err)
	}
	partSize := s.partSizeBytes
	if partSize <= 0 {
		partSize = defaultBackupPartSizeBytes
	}
	if info.Size() <= partSize {
		if _, err := objectStore.UploadFile(ctx, record.S3Key, archivePath, "application/gzip"); err != nil {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), backupObjectCleanupTimeout)
			cleanupErr := deleteBackupObjectKeys(cleanupCtx, objectStore, record)
			cleanupCancel()
			return errors.Join(fmt.Errorf("backup upload: %w", err), cleanupErr)
		}
		record.Parts = nil
		return nil
	}

	localParts, err := splitBackupFile(archivePath, partSize)
	if err != nil {
		return fmt.Errorf("split backup archive: %w", err)
	}
	defer func() {
		paths := make([]string, 0, len(localParts))
		for _, part := range localParts {
			paths = append(paths, part.Path)
		}
		_ = cleanupBackupFiles(paths...)
	}()
	if cfg == nil {
		return errors.New("backup S3 config is unavailable for split upload")
	}

	record.S3Key = ""
	record.Parts = make([]BackupPart, 0, len(localParts))
	partRoot := strings.TrimRight(s.buildS3Key(cfg, record.ID), "/")
	for _, part := range localParts {
		record.Parts = append(record.Parts, BackupPart{
			Index:     part.Index,
			S3Key:     s.buildBackupPartKey(partRoot, part.Index),
			SizeBytes: part.SizeBytes,
			SHA256:    part.SHA256,
		})
	}
	if err := s.saveRecord(ctx, record); err != nil {
		return fmt.Errorf("save split backup plan: %w", err)
	}
	for i, part := range localParts {
		if _, err := objectStore.UploadFile(ctx, record.Parts[i].S3Key, part.Path, "application/octet-stream"); err != nil {
			// PUT 可能已经在对象存储端成功、但客户端因超时收到错误；
			// 因此失败时清理整份分卷计划，而不只清理此前返回成功的卷。
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), backupObjectCleanupTimeout)
			cleanupErr := deleteBackupObjectKeys(cleanupCtx, objectStore, record)
			cleanupCancel()
			return errors.Join(fmt.Errorf("upload backup part %d: %w", part.Index, err), cleanupErr)
		}
	}
	return nil
}

func (s *BackupService) buildBackupPartKey(root string, index int) string {
	return fmt.Sprintf("%s/payload.part-%06d", strings.TrimRight(root, "/"), index)
}

// RestoreBackup is intentionally fail-closed in the online server. A database
// restore must run only after every application slot and worker has stopped;
// an in-process flag cannot fence other blue-green instances or invalidate
// Redis and in-memory state safely.
func (s *BackupService) RestoreBackup(_ context.Context, _ string) error {
	return ErrRestoreRequiresOfflineMaintenance
}

// StartRestore is the HTTP-facing asynchronous entry point and follows the
// same fail-closed policy as RestoreBackup.
func (s *BackupService) StartRestore(_ context.Context, _ string) (*BackupRecord, error) {
	return nil, ErrRestoreRequiresOfflineMaintenance
}

// ─── 备份记录管理 ───

func (s *BackupService) ListBackups(ctx context.Context) ([]BackupRecord, error) {
	records, err := s.loadRecords(ctx)
	if err != nil {
		return nil, err
	}
	// 倒序返回（最新在前）
	sort.Slice(records, func(i, j int) bool {
		return records[i].StartedAt > records[j].StartedAt
	})
	return records, nil
}

func (s *BackupService) GetBackupRecord(ctx context.Context, backupID string) (*BackupRecord, error) {
	records, err := s.loadRecords(ctx)
	if err != nil {
		return nil, err
	}
	for i := range records {
		if records[i].ID == backupID {
			return &records[i], nil
		}
	}
	return nil, ErrBackupNotFound
}

func (s *BackupService) DeleteBackup(ctx context.Context, backupID string) error {
	release, acquired, err := s.acquireLock(ctx, backupOperationLockKey, backupOperationLockTTL)
	if err != nil {
		return err
	}
	if !acquired {
		return ErrBackupInProgress
	}
	defer release()

	var found BackupRecord
	if err := s.withRecordsLock(ctx, func(records []BackupRecord) ([]BackupRecord, bool, error) {
		for i := range records {
			if records[i].ID != backupID {
				continue
			}
			if records[i].Status == "running" {
				// 后台上传仍可能依赖 Parts 计划；删除对象会让随后完成的记录引用失效卷。
				return records, false, ErrBackupInProgress
			}
			found = cloneBackupRecord(records[i])
			return records, false, nil
		}
		return records, false, ErrBackupNotFound
	}); err != nil {
		return err
	}

	// S3 删除可能远超 records lock 的 30 秒租约，因此在短临界区外执行。
	// 删除不完整时保留记录，便于幂等重试。
	if err := s.deleteBackupObjects(ctx, &found); err != nil {
		return err
	}

	return s.withRecordsLock(ctx, func(records []BackupRecord) ([]BackupRecord, bool, error) {
		remaining := make([]BackupRecord, 0, len(records))
		for i := range records {
			if records[i].ID != backupID {
				remaining = append(remaining, records[i])
				continue
			}
			if !backupRecordObjectIdentityEqual(&records[i], &found) {
				return records, false, infraerrors.Conflict(
					"BACKUP_RECORD_CHANGED",
					"backup record changed while its objects were being deleted; retry the operation",
				)
			}
		}
		return remaining, true, nil
	})
}

// GetBackupDownloadURL 获取备份文件预签名下载 URL
func (s *BackupService) GetBackupDownloadURL(ctx context.Context, backupID string) (BackupDownloadResponse, error) {
	var download BackupDownloadResponse
	record, err := s.GetBackupRecord(ctx, backupID)
	if err != nil {
		return download, err
	}
	if record.Status != "completed" {
		return download, infraerrors.BadRequest("BACKUP_NOT_COMPLETED", "backup is not completed")
	}

	s3Cfg, err := s.loadS3Config(ctx)
	if err != nil {
		return download, err
	}
	objectStore, err := s.getOrCreateStore(ctx, s3Cfg)
	if err != nil {
		return download, err
	}

	if len(record.Parts) > 0 {
		parts := append([]BackupPart(nil), record.Parts...)
		sort.Slice(parts, func(i, j int) bool { return parts[i].Index < parts[j].Index })
		for i, part := range parts {
			if part.Index != i+1 || part.S3Key == "" || part.SizeBytes <= 0 {
				return download, fmt.Errorf("invalid backup part metadata at index %d", i+1)
			}
			url, presignErr := objectStore.PresignURL(ctx, part.S3Key, 1*time.Hour)
			if presignErr != nil {
				return download, fmt.Errorf("presign backup part %d: %w", part.Index, presignErr)
			}
			download.Parts = append(download.Parts, BackupDownloadPart{
				Index:     part.Index,
				SizeBytes: part.SizeBytes,
				URL:       url,
			})
		}
		return download, nil
	}
	if record.S3Key == "" {
		return download, errors.New("backup object key is empty")
	}
	url, err := objectStore.PresignURL(ctx, record.S3Key, 1*time.Hour)
	if err != nil {
		return download, fmt.Errorf("presign url: %w", err)
	}
	download.URL = url
	return download, nil
}

// ─── 内部方法 ───

func (s *BackupService) loadS3Config(ctx context.Context) (*BackupS3Config, error) {
	cfg, err := s.loadStoredS3Config(ctx)
	if err != nil || cfg == nil {
		return cfg, err
	}
	// 解密 SecretAccessKey
	if cfg.SecretAccessKey != "" {
		decrypted, decryptErr := s.encryptor.Decrypt(cfg.SecretAccessKey)
		if decryptErr != nil {
			// 兼容未加密的旧数据：如果解密失败，保持原值
			logger.LegacyPrintf("service.backup", "[Backup] S3 SecretAccessKey 解密失败（可能是旧的未加密数据）: %v", decryptErr)
		} else {
			cfg.SecretAccessKey = decrypted
		}
	}
	return cfg, nil
}

// loadStoredS3Config returns the exact persisted representation, including the
// encrypted secret. Callers that update non-secret fields must use this helper
// so they do not write a decrypted secret back to the settings table.
func (s *BackupService) loadStoredS3Config(ctx context.Context) (*BackupS3Config, error) {
	raw, err := s.settingRepo.GetValue(ctx, settingKeyBackupS3Config)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return nil, nil //nolint:nilnil // no config is a valid state
		}
		return nil, fmt.Errorf("load backup s3 config: %w", err)
	}
	if raw == "" {
		return nil, nil //nolint:nilnil // no config is a valid state
	}
	var cfg BackupS3Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, ErrBackupS3ConfigCorrupt
	}
	return &cfg, nil
}

func (s *BackupService) getOrCreateStore(ctx context.Context, cfg *BackupS3Config) (BackupObjectStore, error) {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()

	if s.store != nil && s.s3Cfg != nil && backupS3ConfigEqual(s.s3Cfg, cfg) {
		return s.store, nil
	}

	if cfg == nil {
		return nil, ErrBackupS3NotConfigured
	}

	store, err := s.storeFactory(ctx, cfg)
	if err != nil {
		return nil, err
	}
	s.store = store
	cfgCopy := *cfg
	s.s3Cfg = &cfgCopy
	return store, nil
}

func backupS3StorageLocationEqual(left, right *BackupS3Config) bool {
	if left == nil || right == nil {
		return left == right
	}
	normalizeEndpoint := func(value string) string {
		return strings.TrimRight(strings.TrimSpace(value), "/")
	}
	normalizeRegion := func(value string) string {
		value = strings.TrimSpace(value)
		if value == "" {
			return "auto"
		}
		return value
	}
	return normalizeEndpoint(left.Endpoint) == normalizeEndpoint(right.Endpoint) &&
		normalizeRegion(left.Region) == normalizeRegion(right.Region) &&
		strings.TrimSpace(left.Bucket) == strings.TrimSpace(right.Bucket) &&
		left.ForcePathStyle == right.ForcePathStyle
}

func backupS3ConfigEqual(left, right *BackupS3Config) bool {
	if left == nil || right == nil {
		return left == right
	}
	return backupS3StorageLocationEqual(left, right) &&
		left.AccessKeyID == right.AccessKeyID &&
		left.SecretAccessKey == right.SecretAccessKey &&
		left.Prefix == right.Prefix
}

func (s *BackupService) buildS3Key(cfg *BackupS3Config, fileName string) string {
	prefix := strings.TrimRight(cfg.Prefix, "/")
	if prefix == "" {
		prefix = "backups"
	}
	return fmt.Sprintf("%s/%s/%s", prefix, time.Now().Format("2006/01/02"), fileName)
}

// acquireLock elects one owner for a shared backup operation. Manual and
// record-mutating paths wait for a peer to finish; scheduled/recovery callers
// use the non-blocking form so a second instance simply skips its cycle.
func (s *BackupService) acquireLock(ctx context.Context, key string, ttl time.Duration) (func(), bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	owner := s.instanceID + ":" + uuid.NewString()
	for {
		release, acquired, err := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, key, owner, ttl)
		if err != nil {
			return nil, false, backupCoordinationError(ctx, err)
		}
		if acquired {
			return release, true, nil
		}
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (s *BackupService) tryAcquireLock(ctx context.Context, key string, ttl time.Duration) (func(), bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	owner := s.instanceID + ":" + uuid.NewString()
	release, acquired, err := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, key, owner, ttl)
	if err != nil {
		return nil, false, backupCoordinationError(ctx, err)
	}
	return release, acquired, nil
}

func backupCoordinationError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return ErrBackupCoordinationUnavailable.WithCause(err)
}

// loadRecords 加载备份记录，区分"无数据"和"数据损坏"
func (s *BackupService) loadRecords(ctx context.Context) ([]BackupRecord, error) {
	s.recordsMu.Lock()
	defer s.recordsMu.Unlock()
	return s.loadRecordsLocked(ctx)
}

// loadRecordsLocked 在已持有 recordsMu 锁的情况下加载记录
func (s *BackupService) loadRecordsLocked(ctx context.Context) ([]BackupRecord, error) {
	raw, err := s.settingRepo.GetValue(ctx, settingKeyBackupRecords)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return nil, nil //nolint:nilnil // no records is a valid state
		}
		return nil, fmt.Errorf("load backup records: %w", err)
	}
	if raw == "" {
		return nil, nil //nolint:nilnil // no records is a valid state
	}
	var records []BackupRecord
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return nil, ErrBackupRecordsCorrupt
	}
	return records, nil
}

// saveRecordsLocked 在已持有 recordsMu 锁的情况下保存记录
func (s *BackupService) saveRecordsLocked(ctx context.Context, records []BackupRecord) error {
	data, err := json.Marshal(records)
	if err != nil {
		return err
	}
	return s.settingRepo.Set(ctx, settingKeyBackupRecords, string(data))
}

// saveRecord 保存单条记录（带互斥锁保护）
func (s *BackupService) saveRecord(ctx context.Context, record *BackupRecord) error {
	lockRelease, lockAcquired, err := s.acquireLock(ctx, backupRecordsLockKey, backupRecordsLockTTL)
	if err != nil {
		return err
	}
	if !lockAcquired {
		if ctx != nil && ctx.Err() != nil {
			return fmt.Errorf("acquire backup records lock: %w", ctx.Err())
		}
		return errors.New("acquire backup records lock: another instance is updating records")
	}
	defer lockRelease()
	s.recordsMu.Lock()
	defer s.recordsMu.Unlock()

	records, err := s.loadRecordsLocked(ctx)
	if err != nil {
		return err
	}

	// 更新已有记录或追加
	found := false
	for i := range records {
		if records[i].ID == record.ID {
			records[i] = *record
			found = true
			break
		}
	}
	if !found {
		// Do not silently evict old metadata. RetainCount=0 is documented as
		// unlimited, and every persisted object must keep a record so it remains
		// restorable and deletable. Scheduled retention performs explicit S3
		// deletion after a new backup succeeds.
		records = append(records, *record)
	}

	return s.saveRecordsLocked(ctx, records)
}

// withRecordsLock performs a short read/modify/write transaction on the shared
// records document. The callback must not perform network or object-storage IO.
// It returns the latest record set, whether that set must be persisted, and an
// error. This keeps the Redis lease limited to the setting transaction itself.
func (s *BackupService) withRecordsLock(
	ctx context.Context,
	mutate func([]BackupRecord) ([]BackupRecord, bool, error),
) error {
	release, acquired, err := s.acquireLock(ctx, backupRecordsLockKey, backupRecordsLockTTL)
	if err != nil {
		return err
	}
	if !acquired {
		return ErrBackupInProgress
	}
	defer release()

	s.recordsMu.Lock()
	defer s.recordsMu.Unlock()
	records, err := s.loadRecordsLocked(ctx)
	if err != nil {
		return err
	}
	records, changed, err := mutate(records)
	if err != nil || !changed {
		return err
	}
	return s.saveRecordsLocked(ctx, records)
}

// updateRecordIf conditionally updates an existing record under the shared
// records lock. A missing or concurrently changed record is left untouched;
// recovery paths must never recreate a record that another operation deleted.
func (s *BackupService) updateRecordIf(
	ctx context.Context,
	recordID string,
	matches func(*BackupRecord) bool,
	update func(*BackupRecord),
) (bool, error) {
	updated := false
	err := s.withRecordsLock(ctx, func(records []BackupRecord) ([]BackupRecord, bool, error) {
		for i := range records {
			if records[i].ID != recordID || !matches(&records[i]) {
				continue
			}
			update(&records[i])
			updated = true
			return records, true, nil
		}
		return records, false, nil
	})
	return updated, err
}

func (s *BackupService) cleanupOldBackupsUnlocked(ctx context.Context, schedule *BackupScheduleConfig) error {
	if schedule == nil {
		return nil
	}
	var toDelete []BackupRecord
	if err := s.withRecordsLock(ctx, func(records []BackupRecord) ([]BackupRecord, bool, error) {
		ordered := append([]BackupRecord(nil), records...)
		sort.Slice(ordered, func(i, j int) bool {
			return ordered[i].StartedAt > ordered[j].StartedAt
		})
		for i, r := range ordered {
			shouldDelete := schedule.RetainCount > 0 && i >= schedule.RetainCount
			if schedule.RetainDays > 0 && r.StartedAt != "" {
				startedAt, parseErr := time.Parse(time.RFC3339, r.StartedAt)
				if parseErr == nil && time.Since(startedAt) > time.Duration(schedule.RetainDays)*24*time.Hour {
					shouldDelete = true
				}
			}
			if shouldDelete && r.Status == "completed" {
				toDelete = append(toDelete, cloneBackupRecord(r))
			}
		}
		return records, false, nil
	}); err != nil {
		return err
	}
	if len(toDelete) == 0 {
		return nil
	}

	var cleanupErrs []error
	deleted := make(map[string]BackupRecord, len(toDelete))
	for _, r := range toDelete {
		if err := s.deleteBackupObjects(ctx, &r); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("cleanup backup %s: %w", r.ID, err))
			continue
		}
		deleted[r.ID] = r
	}

	removedCount := 0
	if len(deleted) > 0 {
		candidateRemoved := 0
		if err := s.withRecordsLock(ctx, func(records []BackupRecord) ([]BackupRecord, bool, error) {
			kept := make([]BackupRecord, 0, len(records))
			for i := range records {
				snapshot, ok := deleted[records[i].ID]
				if ok && backupRecordObjectIdentityEqual(&records[i], &snapshot) {
					candidateRemoved++
					continue
				}
				kept = append(kept, records[i])
			}
			return kept, candidateRemoved > 0, nil
		}); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("save backup records after cleanup: %w", err))
		} else {
			removedCount = candidateRemoved
		}
	}
	if removedCount > 0 {
		logger.LegacyPrintf("service.backup", "[Backup] 自动清理了 %d 个过期备份", removedCount)
	}
	return errors.Join(cleanupErrs...)
}

func cloneBackupRecord(record BackupRecord) BackupRecord {
	record.Parts = append([]BackupPart(nil), record.Parts...)
	return record
}

func backupRecordObjectIdentityEqual(left, right *BackupRecord) bool {
	if left == nil || right == nil || left.ID != right.ID {
		return false
	}
	leftKeys := backupObjectKeys(left)
	rightKeys := backupObjectKeys(right)
	if len(leftKeys) != len(rightKeys) {
		return false
	}
	sort.Strings(leftKeys)
	sort.Strings(rightKeys)
	for i := range leftKeys {
		if leftKeys[i] != rightKeys[i] {
			return false
		}
	}
	return true
}

// backupObjectKeys 返回一条备份记录关联的全部对象 key。
// 新记录使用 Parts，旧记录使用 S3Key；两者同时存在时也全部返回，便于清理异常残留对象。
func backupObjectKeys(record *BackupRecord) []string {
	if record == nil {
		return nil
	}
	keys := make([]string, 0, len(record.Parts)+1)
	seen := make(map[string]struct{}, len(record.Parts)+1)
	appendKey := func(key string) {
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	appendKey(record.S3Key)
	parts := append([]BackupPart(nil), record.Parts...)
	sort.Slice(parts, func(i, j int) bool { return parts[i].Index < parts[j].Index })
	for _, part := range parts {
		appendKey(part.S3Key)
	}
	return keys
}

// deleteBackupObjects 尝试删除记录关联的所有对象，并聚合删除错误。
func (s *BackupService) deleteBackupObjects(ctx context.Context, record *BackupRecord) error {
	if len(backupObjectKeys(record)) == 0 {
		return nil
	}
	s3Cfg, err := s.loadS3Config(ctx)
	if err != nil {
		return err
	}
	if s3Cfg == nil || !s3Cfg.IsConfigured() {
		// Without the locator/credentials we cannot prove that remote objects
		// were removed. Preserve the record so an operator can restore the
		// configuration and retry instead of silently orphaning data.
		return ErrBackupS3NotConfigured
	}
	objectStore, err := s.getOrCreateStore(ctx, s3Cfg)
	if err != nil {
		return err
	}
	return deleteBackupObjectKeys(ctx, objectStore, record)
}

func deleteBackupObjectKeys(ctx context.Context, objectStore BackupObjectStore, record *BackupRecord) error {
	keys := backupObjectKeys(record)
	if len(keys) == 0 {
		return nil
	}
	var errs []error
	for _, key := range keys {
		if deleteErr := objectStore.Delete(ctx, key); deleteErr != nil {
			errs = append(errs, fmt.Errorf("delete backup object %q: %w", key, deleteErr))
		}
	}
	return errors.Join(errs...)
}
