package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/robfig/cron/v3"
)

const settingKeyBackupArchiveCheckpoint = "backup_monthly_archive_checkpoint"

// Records and the archive checkpoint share a read/modify/write boundary across
// instances, including manual backups and restore status updates. Use the same
// backend consistently; a Redis error must not silently switch to a DB lock
// while another instance still owns the Redis lock.
func (s *BackupService) lockBackupRecordUpdates(ctx context.Context) (context.Context, func(), error) {
	if s.lockCache == nil && s.db != nil && s.db.Stats().MaxOpenConnections == 1 {
		return nil, nil, fmt.Errorf("backup record locking requires Redis or at least two database connections")
	}
	waitCtx, stopWaiting := context.WithTimeout(ctx, 10*time.Second)
	defer stopWaiting()
	release, acquired, err := s.acquireLock(waitCtx, backupRecordsLockKey, backupRecordsLockTTL)
	if err != nil {
		return nil, nil, err
	}
	if !acquired {
		return nil, nil, ErrBackupInProgress
	}
	s.recordsMu.Lock()
	return ctx, func() { s.recordsMu.Unlock(); release() }, nil
}

type BackupMonthlyArchiveConfig struct {
	Enabled         bool  `json:"enabled"`
	Days            []int `json:"days"`
	IncludeMonthEnd bool  `json:"include_month_end"`
	RetainCount     int   `json:"retain_count"` // 0 = permanent; positive = finite archive pool size.
}

type BackupMonthlyArchive struct {
	Dates       []string `json:"dates"`        // Deduplicated target dates covered by this backup.
	RetainCount int      `json:"retain_count"` // 0 is permanent and never implicitly downgraded.
}

func validateBackupRetention(cfg *BackupScheduleConfig) error {
	if cfg.RetainDays < 0 || cfg.RetainCount < 0 {
		return infraerrors.BadRequest("INVALID_BACKUP_RETENTION", "backup retention must not be negative")
	}
	archive := cfg.MonthlyArchive
	if archive == nil {
		return nil
	}
	if archive.RetainCount < 0 || len(archive.Days) > 31 {
		return infraerrors.BadRequest("INVALID_MONTHLY_ARCHIVE", "invalid archive retention or dates")
	}
	seen := make(map[int]bool)
	days := make([]int, 0, len(archive.Days))
	for _, day := range archive.Days {
		if day < 1 || day > 31 {
			return infraerrors.BadRequest("INVALID_MONTHLY_ARCHIVE", "archive dates must be between 1 and 31")
		}
		if !seen[day] {
			days = append(days, day)
			seen[day] = true
		}
	}
	sort.Ints(days)
	archive.Days = days
	if archive.Enabled && len(days) == 0 && !archive.IncludeMonthEnd {
		return infraerrors.BadRequest("INVALID_MONTHLY_ARCHIVE", "select at least one archive date")
	}
	return nil
}

// assignMonthlyArchive runs under recordsMu and only for a completed scheduled
// backup. Successful processing advances a durable date checkpoint even when
// no date is due. Removing a record or reducing retention cannot reset it.
func (s *BackupService) assignMonthlyArchive(ctx context.Context, record *BackupRecord, schedule *BackupScheduleConfig) (string, error) {
	if record.Status != "completed" || record.TriggeredBy != "scheduled" || record.MonthlyArchive != nil || schedule == nil || schedule.MonthlyArchive == nil || !schedule.MonthlyArchive.Enabled {
		return "", nil
	}
	if err := validateBackupRetention(schedule); err != nil {
		return "", err
	}
	startedAt, err := time.Parse(time.RFC3339, record.StartedAt)
	if err != nil {
		return "", fmt.Errorf("parse archive backup start: %w", err)
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	spec, err := parser.Parse(schedule.CronExpr)
	if err != nil {
		return "", fmt.Errorf("parse archive schedule: %w", err)
	}
	location := time.Local
	if parsed, ok := spec.(*cron.SpecSchedule); ok {
		location = parsed.Location
	}
	startedAt = startedAt.In(location)
	today := startedAt.Format(time.DateOnly)
	checkpoint, err := s.settingRepo.GetValue(ctx, settingKeyBackupArchiveCheckpoint)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return "", fmt.Errorf("load archive checkpoint: %w", err)
	}
	if checkpoint != "" {
		if _, err := time.Parse(time.DateOnly, checkpoint); err != nil {
			return "", infraerrors.InternalServer("BACKUP_ARCHIVE_CHECKPOINT_CORRUPT", "archive checkpoint data is corrupted")
		}
		if checkpoint >= today {
			return "", nil
		}
	}
	dates := monthlyArchiveDates(startedAt, schedule.MonthlyArchive)
	var due []string
	for _, date := range dates {
		if date > checkpoint && date <= today {
			due = append(due, date)
		}
	}
	if len(due) > 0 {
		record.MonthlyArchive = &BackupMonthlyArchive{Dates: due, RetainCount: schedule.MonthlyArchive.RetainCount}
		record.ExpiresAt = ""
	}
	return today, nil
}

func monthlyArchiveDates(at time.Time, cfg *BackupMonthlyArchiveConfig) []string {
	lastDay := time.Date(at.Year(), at.Month()+1, 0, 0, 0, 0, 0, at.Location()).Day()
	unique := make(map[int]bool)
	for _, day := range cfg.Days {
		unique[min(day, lastDay)] = true
	}
	if cfg.IncludeMonthEnd {
		unique[lastDay] = true
	}
	dates := make([]string, 0, len(unique))
	for day := range unique {
		dates = append(dates, time.Date(at.Year(), at.Month(), day, 0, 0, 0, 0, at.Location()).Format(time.DateOnly))
	}
	sort.Strings(dates)
	return dates
}

func backupStartedAfter(a, b BackupRecord) bool {
	left, leftErr := time.Parse(time.RFC3339, a.StartedAt)
	right, rightErr := time.Parse(time.RFC3339, b.StartedAt)
	if leftErr == nil && rightErr == nil && !left.Equal(right) {
		return left.After(right)
	}
	if a.StartedAt != b.StartedAt {
		return a.StartedAt > b.StartedAt
	}
	return a.ID > b.ID
}
