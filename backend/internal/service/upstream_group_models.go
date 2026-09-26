package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

type upstreamGroupModelFetcher interface {
	FetchUpstreamGroupSupportedModels(context.Context, *Account) ([]string, error)
}

type UpstreamGroupModels struct {
	AccountResults     []UpstreamGroupModelAccountResult `json:"-"`
	SourceCount        int                               `json:"source_count"`
	ReadySourceCount   int                               `json:"ready_source_count"`
	PendingSourceCount int                               `json:"pending_source_count"`
	FailedSourceCount  int                               `json:"failed_source_count"`
	StaleSourceCount   int                               `json:"stale_source_count"`
	RefreshInProgress  bool                              `json:"refresh_in_progress"`
	Models             []string                          `json:"models"`
	AutoTags           []string                          `json:"auto_tags"`
	Source             string                            `json:"source"`
	Coverage           string                            `json:"coverage"`
	Status             string                            `json:"status"`
	ErrorCode          string                            `json:"error_code"`
	ObservedAt         *time.Time                        `json:"observed_at"`
	FreshUntil         *time.Time                        `json:"fresh_until"`
}

// Account and its configuration fingerprint are read in the same DB snapshot.
// Neither credentials nor internal source fingerprints are returned to clients.
type UpstreamGroupModelAccountSource struct {
	Account        *Account
	Fingerprint    string
	KeyFingerprint string
}

type UpstreamGroupModelAccountResult struct {
	AccountID   int64
	Fingerprint string
	Models      []string
	AutoTags    []string
	Success     bool
}

type UpstreamGroupModelRepository interface {
	GetGroupModels(context.Context, UpstreamGroupReference) (*UpstreamGroupModels, error)
	ListDueGroupModels(context.Context, time.Time, int) ([]UpstreamGroupReference, error)
	ClaimGroupModels(context.Context, UpstreamGroupReference, int64, string, time.Time, bool) (bool, bool, error)
	SaveGroupModels(context.Context, UpstreamGroupReference, int64, string, UpstreamGroupModels, time.Time) (bool, error)
	ListGroupModelAccountSources(context.Context, UpstreamGroupReference, int64, time.Time, int, bool) ([]UpstreamGroupModelAccountSource, error)
}

func validUpstreamGroupReference(ref UpstreamGroupReference) bool {
	return ref.ConnectionID > 0 && ref.RemoteKey != "" && len(ref.RemoteKey) <= 1024
}

func upstreamGroupRemoteKey(g UpstreamGroup) string {
	if g.RemoteID != "" {
		return "id:" + g.RemoteID
	}
	return "name:" + g.Name
}

func (s *UpstreamConnectionService) GetGroupModels(ctx context.Context, ref UpstreamGroupReference) (*UpstreamGroupModels, error) {
	if !validUpstreamGroupReference(ref) {
		return nil, infraerrors.BadRequest("INVALID_GROUP", "Invalid group reference")
	}
	repo, ok := s.repo.(UpstreamGroupModelRepository)
	if !ok {
		return nil, errors.New("upstream model repository unavailable")
	}
	return repo.GetGroupModels(ctx, ref)
}

// SyncDueGroupModels has its own due queue: frequently refreshed wallet/group
// observations must not cause model-list requests at the same cadence.
func (s *UpstreamConnectionService) SyncDueGroupModels(ctx context.Context) error {
	repo, ok := s.repo.(UpstreamGroupModelRepository)
	if !ok || s.modelFetcher == nil {
		return nil
	}
	refs, err := repo.ListDueGroupModels(ctx, s.now(), 2)
	if err != nil {
		return err
	}
	var syncErrors []error
	for _, ref := range refs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, err := s.SyncGroupModels(ctx, ref, false); err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("connection %d model refresh: %w", ref.ConnectionID, err))
		}
	}
	return errors.Join(syncErrors...)
}

func (s *UpstreamConnectionService) SyncGroupModels(ctx context.Context, ref UpstreamGroupReference, force bool) (*UpstreamGroupModels, error) {
	if !validUpstreamGroupReference(ref) {
		return nil, infraerrors.BadRequest("INVALID_GROUP", "Invalid group reference")
	}
	repo, ok := s.repo.(UpstreamGroupModelRepository)
	if !ok {
		return nil, errors.New("upstream model repository unavailable")
	}
	select {
	case s.modelSyncSlots <- struct{}{}:
		defer func() { <-s.modelSyncSlots }()
	default:
		return nil, infraerrors.Conflict("MODEL_SYNC_BUSY", "Model refresh is busy; retry shortly")
	}
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	connection, err := s.repo.GetByID(ctx, ref.ConnectionID)
	if err != nil {
		return nil, err
	}
	var group *UpstreamGroup
	for i := range connection.Groups {
		if upstreamGroupRemoteKey(connection.Groups[i]) == ref.RemoteKey {
			group = &connection.Groups[i]
			break
		}
	}
	if group == nil {
		return nil, ErrUpstreamConnectionChanged
	}
	token := uuid.NewString()
	claimed, manualBatch, err := repo.ClaimGroupModels(ctx, ref, connection.Version, token, s.now(), force)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, infraerrors.Conflict("MODEL_SYNC_BUSY", "Model refresh is running or was recently completed")
	}
	snapshot := UpstreamGroupModels{Models: []string{}, AutoTags: []string{}, Status: "error", Coverage: "unknown", ErrorCode: "unavailable"}
	// Only authoritative per-group catalogs can claim full group coverage.
	managedCtx, managedCancel := context.WithTimeout(ctx, 15*time.Second)
	models, source, fetchErr := s.fetchManagedGroupModels(managedCtx, connection, *group, manualBatch)
	managedCancel()
	if fetchErr == nil {
		snapshot.Models, snapshot.Source, snapshot.Coverage, snapshot.Status, snapshot.ErrorCode = models, source, "published", "ready", ""
	} else {
		if err := s.fetchBoundGroupModels(ctx, ref, connection.Version, force, &snapshot); err != nil {
			snapshot.ErrorCode = "unavailable"
		}
	}
	if snapshot.Status == "ready" || snapshot.Status == "partial" {
		if !validUpstreamModelSnapshot(snapshot.Models) {
			snapshot.Status = "error"
			snapshot.ErrorCode = "unavailable"
			snapshot.Models = []string{}
			snapshot.AutoTags = []string{}
		} else {
			snapshot.AutoTags = classifyUpstreamGroupModels(snapshot.Models)
		}
	}
	// Persist a failed attempt even if the caller disconnected; never leave a
	// cancelled request holding a lease until expiry without recording its state.
	saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer saveCancel()
	saved, err := repo.SaveGroupModels(saveCtx, ref, connection.Version, token, snapshot, s.now())
	if err != nil {
		return nil, err
	}
	if !saved {
		return nil, ErrUpstreamConnectionChanged
	}
	return repo.GetGroupModels(saveCtx, ref)
}

func (s *UpstreamConnectionService) fetchBoundGroupModels(ctx context.Context, ref UpstreamGroupReference, version int64, force bool, snapshot *UpstreamGroupModels) error {
	snapshot.Source, snapshot.Coverage = "bound_keys", "bound_keys"
	if s.modelFetcher == nil {
		return errors.New("model fetcher unavailable")
	}
	repo, ok := s.repo.(UpstreamGroupModelRepository)
	if !ok {
		return errors.New("model repository unavailable")
	}
	// This is a per-round budget, not a truncation of the group. The repository
	// prioritizes missing/changed and then least-recently-attempted sources.
	sources, err := repo.ListGroupModelAccountSources(ctx, ref, version, s.now(), 8, force)
	if err != nil {
		return err
	}
	for _, source := range sources {
		if ctx.Err() != nil {
			break
		}
		account := source.Account
		if account == nil {
			continue
		}
		result := UpstreamGroupModelAccountResult{AccountID: account.ID, Fingerprint: source.Fingerprint}
		key := upstreamConnectionAPIKey(account)
		if key != "" && upstreamAPIKeyFingerprint(key) == source.KeyFingerprint {
			callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			models, fetchErr := s.modelFetcher.FetchUpstreamGroupSupportedModels(callCtx, account)
			cancel()
			if fetchErr == nil && validUpstreamModelSnapshot(models) {
				result.Success = true
				result.Models = dedupeAndSortModelIDs(models)
				result.AutoTags = classifyUpstreamGroupModels(result.Models)
			}
		}
		snapshot.AccountResults = append(snapshot.AccountResults, result)
	}
	// Aggregate status is computed from current per-source results by the reader.
	snapshot.Status, snapshot.ErrorCode = "pending", "pending_keys"
	return nil
}

func validUpstreamModelSnapshot(models []string) bool {
	if len(models) > 10000 {
		return false
	}
	size := 0
	for _, model := range models {
		if len(model) > 512 || strings.ContainsAny(model, "\x00\r\n") {
			return false
		}
		size += len(model)
	}
	return size <= 1<<20
}

// Classification is deterministic and conservative. Input-image support is
// deliberately not treated as image generation, and unknown IDs get no label.
func classifyUpstreamGroupModels(models []string) []string {
	tags := []string{}
	for _, id := range models {
		name := strings.ToLower(id)
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		for _, rule := range []struct{ prefix, tag string }{
			{"gpt-", "GPT"}, {"o1-", "GPT"}, {"o3-", "GPT"}, {"o4-", "GPT"}, {"chatgpt-", "GPT"},
			{"claude-", "Claude"}, {"gemini-", "Gemini"}, {"grok-", "Grok"}, {"deepseek-", "DeepSeek"},
			{"qwen", "Qwen"}, {"kimi-", "Kimi"}, {"glm-", "GLM"}, {"minimax-", "MiniMax"},
		} {
			if strings.HasPrefix(name, rule.prefix) {
				tags = append(tags, rule.tag)
				break
			}
		}
		if name == "o1" || name == "o3" || name == "o4" {
			tags = append(tags, "GPT", "Text")
		}
		switch {
		case strings.Contains(name, "rerank"):
			tags = append(tags, "Rerank")
		case strings.HasPrefix(name, "text-embedding-"), strings.HasPrefix(name, "bge-"), strings.Contains(name, "-embedding"):
			tags = append(tags, "Embedding")
		case strings.HasPrefix(name, "sora-"), strings.HasPrefix(name, "veo-"), strings.HasPrefix(name, "grok-imagine-video"):
			tags = append(tags, "Video")
		case isImageGenerationModel(name), isOpenAIImageGenerationModel(name), strings.HasPrefix(name, "dall-e-"), strings.HasPrefix(name, "flux-"), strings.HasPrefix(name, "flux."), strings.HasPrefix(name, "imagen-"):
			tags = append(tags, "Image")
		case strings.HasPrefix(name, "whisper-"), strings.HasPrefix(name, "tts-"), strings.Contains(name, "-audio"), strings.Contains(name, "-tts"), strings.Contains(name, "-transcribe"), strings.Contains(name, "-realtime"):
			tags = append(tags, "Audio")
		default:
			if strings.HasPrefix(name, "gpt-4") || strings.HasPrefix(name, "gpt-5") || strings.HasPrefix(name, "claude-") || strings.HasPrefix(name, "deepseek-") || strings.HasPrefix(name, "gemini-") || strings.HasPrefix(name, "grok-3") || strings.HasPrefix(name, "grok-4") {
				tags = append(tags, "Text")
			}
		}
	}
	return dedupeAndSortModelIDs(tags)
}
