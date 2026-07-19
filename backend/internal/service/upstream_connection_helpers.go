package service

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const upstreamConnectionBalancePersistTimeout = 5 * time.Second

func upstreamConnectionAPIKey(account *Account) string {
	if account == nil {
		return ""
	}
	for _, key := range []string{"api_key", "key", "token", "access_token"} {
		if value := strings.TrimSpace(account.GetCredential(key)); value != "" {
			return value
		}
	}
	return ""
}

func upstreamConnectionJoinEndpoint(baseURL string, endpointPath string, keepV1 bool) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	endpointPath = "/" + strings.TrimLeft(endpointPath, "/")
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL + endpointPath
	}
	path := strings.TrimRight(parsed.Path, "/")
	if !keepV1 && strings.HasSuffix(path, "/v1") {
		path = strings.TrimSuffix(path, "/v1")
	}
	if keepV1 && strings.HasPrefix(endpointPath, "/v1/") && strings.HasSuffix(path, "/v1") {
		endpointPath = strings.TrimPrefix(endpointPath, "/v1")
	}
	parsed.Path = path + endpointPath
	return parsed.String()
}

func upstreamConnectionDataObject(payload map[string]any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	if data, ok := payload["data"].(map[string]any); ok {
		return data
	}
	if data, ok := payload["result"].(map[string]any); ok {
		return data
	}
	return payload
}

func upstreamConnectionNumber(data map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok || raw == nil {
			continue
		}
		if value, ok := parseUpstreamConnectionNumber(raw); ok {
			return &value
		}
	}
	return nil
}

func parseUpstreamConnectionNumber(raw any) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, false
		}
		return value, true
	case float32:
		number := float64(value)
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return 0, false
		}
		return number, true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case json.Number:
		number, err := value.Float64()
		return number, err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
	case string:
		normalized := strings.ReplaceAll(strings.TrimSpace(strings.TrimPrefix(value, "$")), ",", "")
		if normalized == "" {
			return 0, false
		}
		number, err := strconv.ParseFloat(normalized, 64)
		return number, err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
	default:
		return 0, false
	}
}

func upstreamConnectionBalancePersistContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), upstreamConnectionBalancePersistTimeout)
}
