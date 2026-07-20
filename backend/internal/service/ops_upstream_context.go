package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Gin context keys used by Ops error logger for capturing upstream error details.
// These keys are set by gateway services and consumed by handler/ops_error_logger.go.
const (
	OpsUpstreamStatusCodeKey   = "ops_upstream_status_code"
	OpsUpstreamErrorMessageKey = "ops_upstream_error_message"
	OpsUpstreamErrorDetailKey  = "ops_upstream_error_detail"
	OpsUpstreamErrorsKey       = "ops_upstream_errors"

	// Best-effort capture of the current upstream request body so ops can
	// inspect the specific upstream attempt rather than only the client request.
	OpsUpstreamRequestBodyKey = "ops_upstream_request_body"

	// Optional stage latencies (milliseconds) for troubleshooting and alerting.
	OpsAuthLatencyMsKey      = "ops_auth_latency_ms"
	OpsRoutingLatencyMsKey   = "ops_routing_latency_ms"
	OpsUpstreamLatencyMsKey  = "ops_upstream_latency_ms"
	OpsResponseLatencyMsKey  = "ops_response_latency_ms"
	OpsTimeToFirstTokenMsKey = "ops_time_to_first_token_ms"
	// OpenAI WS 关键观测字段
	OpsOpenAIWSQueueWaitMsKey = "ops_openai_ws_queue_wait_ms"
	OpsOpenAIWSConnPickMsKey  = "ops_openai_ws_conn_pick_ms"
	OpsOpenAIWSConnReusedKey  = "ops_openai_ws_conn_reused"
	OpsOpenAIWSConnIDKey      = "ops_openai_ws_conn_id"

	// OpsSkipPassthroughKey 由 applyErrorPassthroughRule 在命中 skip_monitoring=true 的规则时设置。
	// ops_error_logger 中间件检查此 key，为 true 时跳过错误记录。
	OpsSkipPassthroughKey = "ops_skip_passthrough"

	// OpsStreamErrorKey 保存 handleStreamingAwareError 在「响应已固化为 HTTP 200 的 SSE 流」
	// 上就地(in-band)补发错误帧时记录的 OpsStreamError。因为 wire 状态码停留在 200，
	// ops_error_logger 的 status>=400 采集路径永远不会触发，这类流内失败
	//（例如等待并发槽位超时后回退的限流、Wait 后二次计费校验失败）本会在错误看板里隐形。
	OpsStreamErrorKey = "ops_stream_error"

	// Client-side configuration denials should remain visible in ops_error_logs,
	// but should be excluded from SLA/error-rate calculations.
	// ResponseCommittedKey 由 handleErrorResponse 系列函数在写完 HTTP 错误响应后设置。
	// ensureForwardErrorResponse 检查此 key，为 true 时跳过兜底写入，避免在已完成的 JSON 后追加 SSE。
	ResponseCommittedKey = "response_committed"

	OpsClientBusinessLimitedKey                          = "ops_client_business_limited"
	OpsClientBusinessLimitedReasonKey                    = "ops_client_business_limited_reason"
	OpsClientBusinessLimitedReasonIPRestriction          = "api_key_ip_restriction"
	OpsClientBusinessLimitedReasonAPIKeyGroupUnavailable = "api_key_group_unavailable"
	OpsClientBusinessLimitedReasonAPIKeyGroupUnassigned  = "api_key_group_unassigned"
	OpsClientBusinessLimitedReasonLocalFeatureGate       = "local_feature_gate"
	OpsClientBusinessLimitedReasonLocalPolicyDenied      = "local_policy_denied"
)

type opsHTTPTraceContextKey struct{}
type opsGzipUpstreamAllowedContextKey struct{}
type openAIHTTPProtocolOverrideContextKey struct{}

// OpsHTTPTrace captures client-side net/http timing for an upstream request.
// Durations are measured from the moment the gateway starts the upstream attempt.
type OpsHTTPTrace struct {
	start time.Time

	DNSStartMs int64
	DNSDoneMs  int64

	ConnectStartMs int64
	ConnectDoneMs  int64

	TLSHandshakeStartMs int64
	TLSHandshakeDoneMs  int64

	GotConnMs       int64
	GotConnReused   bool
	GotConnWasIdle  bool
	GotConnIdleTime int64

	WroteRequestMs    int64
	WroteRequestError string

	GotFirstResponseByteMs int64

	GzipStatus          string
	GzipOriginalBytes   int64
	GzipCompressedBytes int64
	GzipCompressMs      int64
	GzipLevel           int
}

func NewOpsHTTPTrace(start time.Time) *OpsHTTPTrace {
	if start.IsZero() {
		start = time.Now()
	}
	return &OpsHTTPTrace{start: start}
}

func ContextWithOpsHTTPTrace(ctx context.Context, trace *OpsHTTPTrace) context.Context {
	if ctx == nil || trace == nil {
		return ctx
	}
	return context.WithValue(ctx, opsHTTPTraceContextKey{}, trace)
}

func OpsHTTPTraceFromContext(ctx context.Context) *OpsHTTPTrace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(opsHTTPTraceContextKey{}).(*OpsHTTPTrace)
	return trace
}

func ContextWithOpsGzipUpstreamAllowed(ctx context.Context, allowed bool) context.Context {
	if ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, opsGzipUpstreamAllowedContextKey{}, allowed)
}

func ContextWithAccountUpstreamPolicy(ctx context.Context, account *Account) context.Context {
	ctx = ContextWithOpsGzipUpstreamAllowed(ctx, account.IsUpstreamGzipEnabled())
	return ContextWithOpenAIHTTPProtocolOverride(ctx, account.OpenAIHTTPProtocolOverride())
}

func ContextWithAccountUpstreamGzipPolicy(ctx context.Context, account *Account) context.Context {
	return ContextWithAccountUpstreamPolicy(ctx, account)
}

func OpsGzipUpstreamAllowedFromContext(ctx context.Context) (bool, bool) {
	if ctx == nil {
		return false, false
	}
	allowed, ok := ctx.Value(opsGzipUpstreamAllowedContextKey{}).(bool)
	return allowed, ok
}

func ContextWithOpenAIHTTPProtocolOverride(ctx context.Context, protocol string) context.Context {
	if ctx == nil {
		return ctx
	}
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol != OpenAIHTTPProtocolOverrideH1 && protocol != OpenAIHTTPProtocolOverrideH2 {
		return ctx
	}
	return context.WithValue(ctx, openAIHTTPProtocolOverrideContextKey{}, protocol)
}

func OpenAIHTTPProtocolOverrideFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	protocol, _ := ctx.Value(openAIHTTPProtocolOverrideContextKey{}).(string)
	switch protocol {
	case OpenAIHTTPProtocolOverrideH1, OpenAIHTTPProtocolOverrideH2:
		return protocol
	default:
		return ""
	}
}

func (t *OpsHTTPTrace) MarkDNSStart() {
	t.setOnce(&t.DNSStartMs)
}

func (t *OpsHTTPTrace) MarkDNSDone() {
	t.setOnce(&t.DNSDoneMs)
}

func (t *OpsHTTPTrace) MarkConnectStart() {
	t.setOnce(&t.ConnectStartMs)
}

func (t *OpsHTTPTrace) MarkConnectDone() {
	t.setOnce(&t.ConnectDoneMs)
}

func (t *OpsHTTPTrace) MarkTLSHandshakeStart() {
	t.setOnce(&t.TLSHandshakeStartMs)
}

func (t *OpsHTTPTrace) MarkTLSHandshakeDone() {
	t.setOnce(&t.TLSHandshakeDoneMs)
}

func (t *OpsHTTPTrace) MarkGotConn(reused, wasIdle bool, idleTime time.Duration) {
	t.setOnce(&t.GotConnMs)
	t.GotConnReused = reused
	t.GotConnWasIdle = wasIdle
	if idleTime > 0 {
		t.GotConnIdleTime = idleTime.Milliseconds()
	}
}

func (t *OpsHTTPTrace) MarkWroteRequest(err error) {
	t.setOnce(&t.WroteRequestMs)
	if err != nil {
		t.WroteRequestError = sanitizeUpstreamErrorMessage(err.Error())
	}
}

func (t *OpsHTTPTrace) MarkGotFirstResponseByte() {
	t.setOnce(&t.GotFirstResponseByteMs)
}

func (t *OpsHTTPTrace) RecordGzipUpstream(status string, originalBytes, compressedBytes, compressMs int64, level int) {
	if t == nil {
		return
	}
	t.GzipStatus = sanitizeUpstreamErrorMessage(strings.TrimSpace(status))
	t.GzipOriginalBytes = originalBytes
	t.GzipCompressedBytes = compressedBytes
	t.GzipCompressMs = compressMs
	t.GzipLevel = level
}

func (t *OpsHTTPTrace) setOnce(dst *int64) {
	if t == nil || dst == nil || *dst > 0 {
		return
	}
	*dst = time.Since(t.start).Milliseconds()
}

func setOpsUpstreamRequestBody(c *gin.Context, body []byte) {
	if c == nil || len(body) == 0 {
		return
	}
	// 热路径避免 string(body) 额外分配，按需在落库前再转换。
	c.Set(OpsUpstreamRequestBodyKey, body)
}

func MarkResponseCommitted(c *gin.Context) {
	if c != nil {
		c.Set(ResponseCommittedKey, true)
	}
}

func IsResponseCommitted(c *gin.Context) bool {
	if c == nil {
		return false
	}
	v, ok := c.Get(ResponseCommittedKey)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

func SetOpsLatencyMs(c *gin.Context, key string, value int64) {
	if c == nil || strings.TrimSpace(key) == "" || value < 0 {
		return
	}
	c.Set(key, value)
}

func MarkOpsClientBusinessLimited(c *gin.Context, reason string) {
	if c == nil {
		return
	}
	c.Set(OpsClientBusinessLimitedKey, true)
	if reason = strings.TrimSpace(reason); reason != "" {
		c.Set(OpsClientBusinessLimitedReasonKey, reason)
	}
}

func HasOpsClientBusinessLimited(c *gin.Context) bool {
	if c == nil {
		return false
	}
	v, ok := c.Get(OpsClientBusinessLimitedKey)
	if !ok {
		return false
	}
	marked, _ := v.(bool)
	return marked
}

// OpsStreamError 描述网关在「响应状态已固化为 200」之后（keepalive ping 或部分数据
// 已 flush）就地以 SSE error 帧形式返回的错误。由于 HTTP 状态码停留在 200，
// 而 ops_error_logger 以 status>=400 为采集触发条件，这类流内失败
// （并发限流回退、Wait 后二次计费校验失败、流开始后才无可用账号等）本会在错误看板里
// 完全隐形。handler.handleStreamingAwareError 负责标记，ops_error_logger 中间件在
// status<400 分支消费它并补记一条错误日志。
type OpsStreamError struct {
	// ErrType 是写入 SSE 帧的对客错误类型（如 rate_limit_error / upstream_error / api_error）。
	ErrType string
	// Code 是可选的稳定错误分类；用于既保留通用 OpenAI error.type，又向客户端和 Ops
	// 暴露可编程判断的细分类（如 upstream_http2_stream_error）。
	Code string
	// Message 是写入 SSE 帧的对客错误消息。
	Message string
	// IntendedStatus 是流若未固化本应返回的 HTTP 状态码（如并发限流的 429）。
	// 默认仅用于错误分级；CountTowardsSLA=true 时也作为 Ops 的逻辑状态码。
	IntendedStatus int
	// CountTowardsSLA 表示虽然 wire 状态已固化为 200，请求在应用语义上仍然失败，
	// Ops 应使用 IntendedStatus 计入错误率/SLA。
	CountTowardsSLA bool
}

// MarkOpsStreamError 记录一次就地 SSE 错误，供 ops 日志采集。
// 采用「首个标记生效」策略：同一请求若先后补发多帧（如上游透传错误后又追加通用兜底帧），
// 保留最先记录的根因错误，而不是被后续的 "Upstream request failed" 覆盖。
func MarkOpsStreamError(c *gin.Context, errType, message string, intendedStatus int) {
	markOpsStreamError(c, OpsStreamError{
		ErrType:        errType,
		Message:        message,
		IntendedStatus: intendedStatus,
	})
}

// MarkOpsStreamFailure records an in-band stream error that represents a failed
// request and therefore must count towards Ops error rate/SLA despite HTTP 200
// already being committed on the wire.
func MarkOpsStreamFailure(c *gin.Context, errType, code, message string, intendedStatus int) {
	markOpsStreamError(c, OpsStreamError{
		ErrType:         errType,
		Code:            code,
		Message:         message,
		IntendedStatus:  intendedStatus,
		CountTowardsSLA: true,
	})
}

func markOpsStreamError(c *gin.Context, streamErr OpsStreamError) {
	if c == nil {
		return
	}
	if _, exists := c.Get(OpsStreamErrorKey); exists {
		return
	}
	streamErr.ErrType = strings.TrimSpace(streamErr.ErrType)
	streamErr.Code = strings.TrimSpace(streamErr.Code)
	streamErr.Message = strings.TrimSpace(streamErr.Message)
	c.Set(OpsStreamErrorKey, streamErr)
}

// GetOpsStreamError 返回本请求记录的就地 SSE 错误（若有）。
func GetOpsStreamError(c *gin.Context) (OpsStreamError, bool) {
	if c == nil {
		return OpsStreamError{}, false
	}
	v, ok := c.Get(OpsStreamErrorKey)
	if !ok {
		return OpsStreamError{}, false
	}
	se, ok := v.(OpsStreamError)
	return se, ok
}

// SetOpsUpstreamError is the exported wrapper for setOpsUpstreamError, used by
// handler-layer code (e.g. failover-exhausted paths) that needs to record the
// original upstream status code before mapping it to a client-facing code.
func SetOpsUpstreamError(c *gin.Context, upstreamStatusCode int, upstreamMessage, upstreamDetail string) {
	setOpsUpstreamError(c, upstreamStatusCode, upstreamMessage, upstreamDetail)
}

func setOpsUpstreamError(c *gin.Context, upstreamStatusCode int, upstreamMessage, upstreamDetail string) {
	if c == nil {
		return
	}
	if upstreamStatusCode > 0 {
		c.Set(OpsUpstreamStatusCodeKey, upstreamStatusCode)
	}
	if msg := strings.TrimSpace(upstreamMessage); msg != "" {
		c.Set(OpsUpstreamErrorMessageKey, msg)
	}
	if detail := strings.TrimSpace(upstreamDetail); detail != "" {
		c.Set(OpsUpstreamErrorDetailKey, detail)
	}
}

// OpsUpstreamErrorEvent describes one upstream error attempt during a single gateway request.
// It is stored in ops_error_logs.upstream_errors as a JSON array.
type OpsUpstreamErrorEvent struct {
	AtUnixMs int64 `json:"at_unix_ms,omitempty"`

	// Passthrough 表示本次请求是否命中“原样透传（仅替换认证）”分支。
	// 该字段用于排障与灰度评估；存入 JSON，不涉及 DB schema 变更。
	Passthrough bool `json:"passthrough,omitempty"`

	// Context
	Platform    string `json:"platform,omitempty"`
	AccountID   int64  `json:"account_id,omitempty"`
	AccountName string `json:"account_name,omitempty"`

	// Outcome
	UpstreamStatusCode int    `json:"upstream_status_code,omitempty"`
	UpstreamRequestID  string `json:"upstream_request_id,omitempty"`

	// UpstreamURL is the actual upstream URL that was called (host + path, query/fragment stripped).
	// Helps debug 404/routing errors by showing which endpoint was targeted.
	UpstreamURL string `json:"upstream_url,omitempty"`

	// Best-effort upstream response capture (sanitized+trimmed).
	UpstreamResponseBody string `json:"upstream_response_body,omitempty"`

	// Kind: http_error | request_error | retry_exhausted | failover
	Kind string `json:"kind,omitempty"`
	// Stage/Scope/Reason distinguish credential acquisition from inference
	// without overloading upstream_status_code with a synthetic HTTP status.
	Stage  string `json:"stage,omitempty"`
	Scope  string `json:"scope,omitempty"`
	Reason string `json:"reason,omitempty"`

	Message string `json:"message,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

func appendOpsUpstreamError(c *gin.Context, ev OpsUpstreamErrorEvent) {
	if c == nil {
		return
	}
	if ev.AtUnixMs <= 0 {
		ev.AtUnixMs = time.Now().UnixMilli()
	}
	ev.Platform = strings.TrimSpace(ev.Platform)
	ev.UpstreamRequestID = strings.TrimSpace(ev.UpstreamRequestID)
	ev.UpstreamResponseBody = strings.TrimSpace(ev.UpstreamResponseBody)
	ev.Kind = strings.TrimSpace(ev.Kind)
	ev.Stage = strings.TrimSpace(ev.Stage)
	ev.Scope = strings.TrimSpace(ev.Scope)
	ev.Reason = strings.TrimSpace(ev.Reason)
	ev.UpstreamURL = strings.TrimSpace(ev.UpstreamURL)
	ev.Message = strings.TrimSpace(ev.Message)
	ev.Detail = strings.TrimSpace(ev.Detail)
	if ev.Message != "" {
		ev.Message = sanitizeUpstreamErrorMessage(ev.Message)
	}

	var existing []*OpsUpstreamErrorEvent
	if v, ok := c.Get(OpsUpstreamErrorsKey); ok {
		if arr, ok := v.([]*OpsUpstreamErrorEvent); ok {
			existing = arr
		}
	}

	evCopy := ev
	existing = append(existing, &evCopy)
	c.Set(OpsUpstreamErrorsKey, existing)

	checkSkipMonitoringForUpstreamEvent(c, &evCopy)
}

// checkSkipMonitoringForUpstreamEvent checks whether the upstream error event
// matches a passthrough rule with skip_monitoring=true and, if so, sets the
// OpsSkipPassthroughKey on the context.  This ensures intermediate retry /
// failover errors (which never go through the final applyErrorPassthroughRule
// path) can still suppress ops_error_logs recording.
func checkSkipMonitoringForUpstreamEvent(c *gin.Context, ev *OpsUpstreamErrorEvent) {
	if ev.UpstreamStatusCode == 0 {
		return
	}

	svc := getBoundErrorPassthroughService(c)
	if svc == nil {
		return
	}

	// Use the best available body representation for keyword matching.
	// Even when body is empty, MatchRule can still match rules that only
	// specify ErrorCodes (no Keywords), so we always call it.
	body := ev.Detail
	if body == "" {
		body = ev.Message
	}

	rule := svc.MatchRule(ev.Platform, ev.UpstreamStatusCode, []byte(body))
	if rule != nil && rule.SkipMonitoring {
		c.Set(OpsSkipPassthroughKey, true)
	}
}

func marshalOpsUpstreamErrors(events []*OpsUpstreamErrorEvent) *string {
	if len(events) == 0 {
		return nil
	}
	// Ensure we always store a valid JSON value.
	raw, err := json.Marshal(events)
	if err != nil || len(raw) == 0 {
		return nil
	}
	s := string(raw)
	return &s
}

func ParseOpsUpstreamErrors(raw string) ([]*OpsUpstreamErrorEvent, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []*OpsUpstreamErrorEvent{}, nil
	}
	var out []*OpsUpstreamErrorEvent
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// safeUpstreamURL returns scheme + host + path from a URL, stripping query/fragment
// to avoid leaking sensitive query parameters (e.g. OAuth tokens).
func safeUpstreamURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if idx := strings.IndexByte(rawURL, '?'); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	if idx := strings.IndexByte(rawURL, '#'); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	return rawURL
}
