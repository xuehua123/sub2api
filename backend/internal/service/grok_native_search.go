package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const grokNativeSearchResponseLimit = 4 << 20

// DoGrokNativeResponsesJSON posts a standalone web/x_search request through
// the same Grok account-health path used by normal Responses traffic.
func (s *OpenAIGatewayService) DoGrokNativeResponsesJSON(ctx context.Context, account *Account, body []byte) ([]byte, error) {
	if s == nil || s.httpUpstream == nil {
		return nil, errors.New("http upstream not configured")
	}
	if account == nil {
		return nil, errors.New("account is required")
	}
	if !account.IsGrok() {
		return nil, errors.New("grok account required")
	}

	body, upstreamModel, err := resolveGrokNativeSearchBody(account, body)
	if err != nil {
		return nil, err
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, &UpstreamFailoverError{
			StatusCode: http.StatusUnauthorized,
			Reason:     GatewayFailureReason("grok_search_token"),
		}
	}

	upstreamReq, err := buildGrokResponsesRequest(ctx, nil, account, body, token, "", s.cfg, s.settingService)
	if err != nil {
		return nil, fmt.Errorf("build grok responses request: %w", err)
	}
	// Standalone search historically uses the pinned CLI identity for both OAuth
	// and API-key accounts. Preserve that wire contract while moving the call
	// onto the account-aware Grok service.
	upstreamReq.Header.Set("Accept", "application/json")
	applyGrokCLIHeaders(upstreamReq.Header)
	account.ApplyHeaderOverrides(upstreamReq.Header)
	upstreamReq = upstreamReq.WithContext(ContextWithAccountUpstreamPolicy(upstreamReq.Context(), account))
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, &UpstreamFailoverError{
			StatusCode:             http.StatusBadGateway,
			Reason:                 GatewayFailureReason("grok_search_transport"),
			RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(http.StatusBadGateway),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, grokNativeSearchResponseLimit))
	if readErr != nil {
		return nil, &UpstreamFailoverError{
			StatusCode:             http.StatusBadGateway,
			Reason:                 GatewayFailureReason("grok_search_read"),
			RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(http.StatusBadGateway),
		}
	}

	stateCtx := withGrokTeamRateLimitModel(ctx, upstreamModel)
	if resp.StatusCode >= http.StatusBadRequest {
		modelUnavailable := isGrokModelProviderUnavailable(resp.StatusCode, respBytes)
		shouldFailover := s.shouldFailoverGrokUpstreamErrorForContext(stateCtx, resp.StatusCode, respBytes)
		shouldDisable := s.handleGrokAccountUpstreamError(stateCtx, account, resp.StatusCode, resp.Header, respBytes)
		if shouldFailover {
			reason := GatewayFailureReason("grok_search_upstream")
			if modelUnavailable {
				reason = GatewayFailureReason("grok_search_model_provider_unavailable")
			}
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBytes,
				ResponseHeaders:        resp.Header.Clone(),
				Reason:                 reason,
				RetryableOnSameAccount: !modelUnavailable && !shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		msg := string(respBytes)
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("grok upstream %d: %s", resp.StatusCode, msg)
	}

	s.updateGrokUsageFromResponse(stateCtx, account, resp.Header, resp.StatusCode)
	return respBytes, nil
}

func resolveGrokNativeSearchBody(account *Account, body []byte) ([]byte, string, error) {
	if !gjson.ValidBytes(body) {
		return nil, "", errors.New("invalid grok search request body")
	}
	requestedModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	upstreamModel := requestedModel
	if account != nil {
		upstreamModel, _ = account.ResolveMappedModel(requestedModel)
	}
	if strings.TrimSpace(upstreamModel) == "" {
		upstreamModel = xai.DefaultTextModel
	}
	upstreamModel = xai.ResolveGrokTextResponsesModelID(upstreamModel, xai.DefaultTextModel)
	patched, err := sjson.SetBytes(body, "model", upstreamModel)
	if err != nil {
		return nil, "", fmt.Errorf("set grok search upstream model: %w", err)
	}
	return patched, upstreamModel, nil
}
