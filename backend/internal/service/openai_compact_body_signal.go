package service

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// openAINativeCompactionV2Key 标记本请求是原生 remote compaction v2
// （裸 /responses + stream:true + compaction_trigger），由 handler 在判定后
// 写入，供上游请求构造时补注协商头。
const openAINativeCompactionV2Key = "openai_native_compaction_v2"

// openAINativeCompactionV2SchedulingContextKey carries the native v2
// capability requirement through account scheduling. It is deliberately
// separate from the legacy /responses/compact flag: native v2 must avoid an
// account that the native probe has explicitly marked unsupported, but must
// not opt into legacy compact_model_mapping semantics.
type openAINativeCompactionV2SchedulingContextKey struct{}

const openAIRemoteCompactionV2Feature = "remote_compaction_v2"

// MarkOpenAINativeCompactionV2 由 handler 在识别出原生 v2 压缩请求时调用。
func MarkOpenAINativeCompactionV2(c *gin.Context) {
	if c != nil {
		c.Set(openAINativeCompactionV2Key, true)
	}
}

// IsOpenAINativeCompactionV2 reports whether the handler recognized the
// current request as a native remote compaction v2 turn.
func IsOpenAINativeCompactionV2(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return c.GetBool(openAINativeCompactionV2Key)
}

// WithOpenAINativeCompactionV2Scheduling marks a request context so account
// selection enforces the native v2 probe result. It does not change legacy
// compact request semantics such as compact_model_mapping.
func WithOpenAINativeCompactionV2Scheduling(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAINativeCompactionV2SchedulingContextKey{}, true)
}

func requiresOpenAINativeCompactionV2Scheduling(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	required, _ := ctx.Value(openAINativeCompactionV2SchedulingContextKey{}).(bool)
	return required
}

// ensureOpenAIRemoteCompactionV2BetaFeature 确保出站 x-codex-beta-features
// 头包含 remote_compaction_v2。真实 Codex 发送 compaction_trigger 时总会同时
// 携带该协商头（codex-rs build_model_client_beta_features_header 对该 feature
// 特判 advertise）；上游或下游网关链剥掉它后，请求会在依赖该头做门控的
// 环节被降级（#5586）。这里在原生 v2 请求出站前补齐，使线型与真实 Codex
// 一致。已存在时保持原样，不重复追加。
func ensureOpenAIRemoteCompactionV2BetaFeature(h http.Header) {
	if h == nil {
		return
	}
	const headerName = "x-codex-beta-features"
	canonicalHeaderName := http.CanonicalHeaderKey(headerName)
	tokens := make([]string, 0, 4)
	matchingKeys := make([]string, 0, 1)
	hasFeature := false
	canonicalOnly := true
	for name, values := range h {
		// Account header overrides deliberately preserve wire casing and can
		// therefore leave this key as lower-case. http.Header.Values only
		// sees the canonical map key, so scan the raw map case-insensitively.
		if !strings.EqualFold(strings.TrimSpace(name), headerName) {
			continue
		}
		matchingKeys = append(matchingKeys, name)
		if name != canonicalHeaderName {
			canonicalOnly = false
		}
		for _, value := range values {
			for _, token := range strings.Split(value, ",") {
				token = strings.TrimSpace(token)
				if token == "" {
					continue
				}
				if strings.EqualFold(token, openAIRemoteCompactionV2Feature) {
					hasFeature = true
				}
				tokens = append(tokens, token)
			}
		}
	}
	// Keep the existing canonical header byte-for-byte when it already
	// advertises v2. Besides avoiding needless churn, this preserves any
	// client-selected token formatting.
	if hasFeature && canonicalOnly && len(matchingKeys) == 1 {
		return
	}
	if !hasFeature {
		tokens = append(tokens, openAIRemoteCompactionV2Feature)
	}
	// Collapse mixed/raw casing to one canonical key. Without this, a raw
	// API-key override can coexist with a canonical v2 header and Go's
	// header lookup/writer may hide or drop one of them.
	for _, name := range matchingKeys {
		delete(h, name)
	}
	h.Set(canonicalHeaderName, strings.Join(tokens, ","))
}

// hasOpenAIRemoteCompactionV2BetaFeature reports whether an already-built
// upstream handshake advertises remote compaction v2. It accepts repeated
// header lines and mixed casing so the check is safe after account-level
// header overrides.
func hasOpenAIRemoteCompactionV2BetaFeature(h http.Header) bool {
	for name, values := range h {
		if !strings.EqualFold(strings.TrimSpace(name), "x-codex-beta-features") {
			continue
		}
		for _, value := range values {
			for _, feature := range strings.Split(value, ",") {
				if strings.EqualFold(strings.TrimSpace(feature), openAIRemoteCompactionV2Feature) {
					return true
				}
			}
		}
	}
	return false
}

// hasOpenAICodexBetaFeaturesHeader 报告出站头里是否已存在非空的
// x-codex-beta-features（即客户端自己声明过能力集）。
func hasOpenAICodexBetaFeaturesHeader(h http.Header) bool {
	if h == nil {
		return false
	}
	for _, value := range h.Values("x-codex-beta-features") {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

// applyOpenAICodexBetaFeatures 按真实 Codex 的会话级行为补注
// x-codex-beta-features。
//
// codex 侧规则（codex-rs：session/mod.rs build_model_client_beta_features_header
// 组装、client.rs build_responses_headers 附加）：该头是**会话级常量**，挂在
// /responses、WS 握手、/responses/compact 三处的**每一个**请求上，而不是只挂
// 压缩回合。其内容是"已启用且需要 advertise 的 feature 列表"，RemoteCompactionV2
// 被特判 advertise；实测默认安装下没有任何 Experimental 特性默认开启，
// 因此默认 Codex 的头值恰好就是单个 "remote_compaction_v2"。
//
// 网关据此对齐：
//   - 原生 v2 压缩回合（body 带 compaction_trigger 实锤）：无论账号类型都确保
//     v2 在列，覆盖中间网关裁剪 token 的情形（#5586）；
//   - ChatGPT codex 上游（OAuth）的其余请求：客户端**未**声明该头时补成默认
//     Codex 形态，消除"仅压缩回合才带该头"这种真实 Codex 不会产生的模式；
//   - 客户端已声明该头：原样保留。非空但不含 v2 表示用户显式关闭了该特性，
//     网关不得替其改写能力声明；
//   - 非 OAuth 上游（API Key/第三方兼容网关）：不做会话级注入，只保留压缩回合
//     的那一条，避免向非 Codex 后端撒 Codex 专属头。
//
// 已知无解的歧义：用户关掉 v2 且无其他特性时，真实 Codex 同样不发该头，与"老
// 客户端"在线型上不可区分，此时按默认形态补注。该用户的 legacy 压缩端点本就
// 已被上游下线（404），不存在可回退的正确行为。
func applyOpenAICodexBetaFeatures(c *gin.Context, account *Account, h http.Header) {
	if h == nil {
		return
	}
	if IsOpenAINativeCompactionV2(c) {
		ensureOpenAIRemoteCompactionV2BetaFeature(h)
		return
	}
	if account == nil || !account.IsOpenAIOAuth() {
		return
	}
	if hasOpenAICodexBetaFeaturesHeader(h) {
		return
	}
	h.Set("x-codex-beta-features", openAIRemoteCompactionV2Feature)
}

// HasCompactionTriggerInInput detects an input item with
// type="compaction_trigger". The handler combines this body signal with the
// request path and stream flag to distinguish the native remote compaction v2
// wire from the legacy /responses/compact bridge.
func HasCompactionTriggerInInput(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "compaction_trigger" {
			found = true
			return false
		}
		return true
	})
	return found
}
