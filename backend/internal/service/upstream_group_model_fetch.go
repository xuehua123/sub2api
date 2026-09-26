package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const upstreamGroupModelMaxPages = 100

// Build requests on an isolated account copy: model observation must never
// rewrite the stored account type/credentials or enter an OAuth refresh path.
func (s *AccountTestService) buildUpstreamGroupModelsRequest(ctx context.Context, account *Account) (*http.Request, *Account, error) {
	if account == nil || !supportsUpstreamConnectionAccountType(account.Type) {
		return nil, nil, newUpstreamModelSyncUnsupportedError("Group model discovery requires an API-key or upstream account", nil)
	}
	if account.Type != AccountTypeUpstream {
		req, err := s.buildUpstreamModelsRequest(ctx, account)
		return req, account, err
	}
	key := upstreamConnectionAPIKey(account)
	base := strings.TrimSpace(account.GetCredential("base_url"))
	if key == "" || base == "" {
		return nil, nil, newUpstreamModelSyncConfigError("Upstream account requires a base URL and API key", nil)
	}
	copyAccount := *account
	copyAccount.Type = AccountTypeAPIKey
	copyAccount.Credentials = make(map[string]any, len(account.Credentials)+1)
	for k, v := range account.Credentials {
		copyAccount.Credentials[k] = v
	}
	copyAccount.Credentials["api_key"] = key
	if account.Platform == PlatformAntigravity {
		// Antigravity upstream accounts forward to a Claude-compatible gateway,
		// not the official OAuth Cloud Code endpoint or /antigravity API-key path.
		validated, err := s.validateUpstreamBaseURL(base)
		if err != nil {
			return nil, nil, newUpstreamModelSyncConfigError("Invalid upstream base URL", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildV1ModelsURL(validated), nil)
		if err != nil {
			return nil, nil, newUpstreamModelSyncConfigError("Invalid model list URL", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
		return req, &copyAccount, nil
	}
	req, err := s.buildUpstreamModelsRequest(ctx, &copyAccount)
	return req, &copyAccount, err
}

// A source is successful only after ALL pages have been validated. Any error,
// cancellation, cursor loop or size limit returns no partial replacement so the
// repository can retain that source's last successful result.
func (s *AccountTestService) fetchUpstreamGroupModelPages(ctx context.Context, account *Account) ([]string, error) {
	if s == nil || s.httpUpstream == nil {
		return nil, newUpstreamModelSyncConfigError("Upstream HTTP client is not configured", nil)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, requestAccount, err := s.buildUpstreamGroupModelsRequest(ctx, account)
	if err != nil {
		return nil, err
	}
	proxy := upstreamModelsProxyURL(requestAccount)
	remaining := min(resolveModelsListReadLimit(s.cfg), upstreamModelsBodyLimit)
	models := []string{}
	seenModels := map[string]struct{}{}
	seenCursors := map[string]struct{}{}
	for _, name := range []string{"pageToken", "after_id"} {
		if cursor := req.URL.Query().Get(name); cursor != "" {
			seenCursors[name+":"+cursor] = struct{}{}
		}
	}
	for page := 0; page < upstreamGroupModelMaxPages; page++ {
		if err := ctx.Err(); err != nil {
			return nil, newUpstreamModelSyncUpstreamError("Model discovery was interrupted", err)
		}
		body, err := s.readUpstreamGroupModelPage(req, requestAccount, proxy, remaining)
		if err != nil {
			return nil, err
		}
		remaining -= int64(len(body))
		entries, queryKey, cursor, err := parseUpstreamGroupModelPage(body, requestAccount.IsGrok())
		if err != nil {
			return nil, newUpstreamModelSyncUpstreamError("Upstream returned an invalid model catalog", err)
		}
		for _, model := range entries {
			if _, exists := seenModels[model]; !exists {
				seenModels[model] = struct{}{}
				models = append(models, model)
			}
		}
		if !validUpstreamModelSnapshot(models) {
			return nil, newUpstreamModelSyncUpstreamError("Upstream model catalog exceeds the supported limit", nil)
		}
		if cursor == "" {
			return dedupeAndSortModelIDs(models), nil
		}
		if len(entries) == 0 {
			return nil, newUpstreamModelSyncUpstreamError("Upstream pagination made no progress", nil)
		}
		cursorKey := queryKey + ":" + cursor
		if _, seen := seenCursors[cursorKey]; seen {
			return nil, newUpstreamModelSyncUpstreamError("Upstream pagination repeated a cursor", nil)
		}
		seenCursors[cursorKey] = struct{}{}
		// Never follow an upstream-provided URL. Change only a cursor parameter
		// on the already validated endpoint, retaining auth, proxy and query data.
		next := req.Clone(ctx)
		query := next.URL.Query()
		query.Set(queryKey, cursor)
		next.URL.RawQuery = query.Encode()
		req = next
	}
	return nil, newUpstreamModelSyncUpstreamError("Upstream model catalog exceeded the page limit", nil)
}

func (s *AccountTestService) readUpstreamGroupModelPage(req *http.Request, account *Account, proxy string, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, newUpstreamModelSyncUpstreamError("Upstream model catalog exceeds the byte limit", nil)
	}
	resp, err := s.doUpstreamModelsRequest(req, proxy, account)
	if err != nil {
		return nil, newUpstreamModelSyncUpstreamError("Failed to request upstream model list", err)
	}
	if resp == nil || resp.Body == nil {
		return nil, newUpstreamModelSyncUpstreamError("Upstream returned an empty HTTP response", nil)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &UpstreamModelSyncError{Kind: UpstreamModelSyncErrorUpstream, Message: fmt.Sprintf("Upstream model list request failed with HTTP %d", resp.StatusCode), StatusCode: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, newUpstreamModelSyncUpstreamError("Failed to read upstream model list", err)
	}
	if int64(len(body)) > limit {
		return nil, newUpstreamModelSyncUpstreamError("Upstream model catalog exceeds the byte limit", nil)
	}
	return body, nil
}

func parseUpstreamGroupModelPage(body []byte, grok bool) ([]string, string, string, error) {
	var envelope map[string]json.RawMessage
	// Bare arrays are valid catalogs; objects must not report business failure.
	if err := json.Unmarshal(body, &envelope); err == nil && envelope != nil {
		if raw, exists := envelope["success"]; exists {
			var success bool
			if json.Unmarshal(raw, &success) != nil || !success {
				return nil, "", "", errors.New("model catalog reports failure")
			}
		}
		if raw, exists := envelope["error"]; exists {
			switch strings.TrimSpace(string(raw)) {
			case "null", "false", `""`:
			default:
				return nil, "", "", errors.New("model catalog reports an error")
			}
		}
		if raw, exists := envelope["code"]; exists {
			var value any
			if json.Unmarshal(raw, &value) != nil {
				return nil, "", "", errors.New("invalid model catalog status code")
			}
			switch value {
			case float64(0), float64(200), "0", "200":
			default:
				return nil, "", "", errors.New("model catalog reports an unsuccessful code")
			}
		}
	}
	entries, err := extractUpstreamModelRawEntries(body)
	if err != nil || entries == nil {
		return nil, "", "", errors.New("missing explicit model list")
	}
	models := []string{}
	for _, raw := range entries {
		var entry upstreamModelEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, "", "", errors.New("invalid model entry")
		}
		id := upstreamModelEntryID(entry)
		if grok {
			id = grokUpstreamModelEntryID(entry)
		}
		if strings.TrimSpace(id) == "" {
			return nil, "", "", errors.New("model entry has no identifier")
		}
		models = append(models, id)
	}
	key, cursor, err := upstreamGroupModelNextPage(envelope)
	return models, key, cursor, err
}

func upstreamGroupModelNextPage(envelope map[string]json.RawMessage) (string, string, error) {
	readCursor := func(key string) (string, error) {
		raw, exists := envelope[key]
		if !exists || string(raw) == "null" {
			return "", nil
		}
		var value string
		if json.Unmarshal(raw, &value) != nil || len(value) > 4096 {
			return "", errors.New("invalid model pagination cursor")
		}
		return value, nil
	}
	token, err := readCursor("nextPageToken")
	if err != nil {
		return "", "", err
	}
	hasMore := false
	if raw, exists := envelope["has_more"]; exists {
		if string(raw) == "null" || json.Unmarshal(raw, &hasMore) != nil {
			return "", "", errors.New("invalid model pagination flag")
		}
	}
	if hasMore {
		last, err := readCursor("last_id")
		if err != nil || strings.TrimSpace(last) == "" || token != "" {
			return "", "", errors.New("missing or conflicting model pagination cursor")
		}
		return "after_id", last, nil
	}
	if token != "" {
		return "pageToken", token, nil
	}
	return "", "", nil
}
