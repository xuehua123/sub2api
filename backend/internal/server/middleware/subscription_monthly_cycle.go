package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Only generation submissions can consume a future cycle. Discovery, token
// counting, billing, polling and cancellation must remain side-effect free.
func isMonthlyCycleBillableRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.Method != http.MethodPost {
		return false
	}
	path := strings.TrimSuffix(c.Request.URL.Path, "/")
	for _, suffix := range []string{"/alpha/search", "/web_search", "/x_search", "/live", "/realtime/calls", "/contents/generations/tasks"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	for _, suffix := range []string{"/messages", "/responses", "/responses/compact", "/chat/completions", "/completions", "/embeddings", "/images/generations", "/images/edits", "/images/generations/async", "/images/edits/async", "/images/batches", "/videos", "/videos/generations", "/videos/edits", "/videos/extensions", "/audio/speech", "/audio/transcriptions", "/audio/translations", "/tts", "/stt"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return strings.HasSuffix(path, ":generateContent") || strings.HasSuffix(path, ":streamGenerateContent")
}

func isMonthlyCycleWebSocketRequest(c *gin.Context) bool {
	if isResponsesWebSocketRoute(c) {
		return true
	}
	if c == nil || c.Request == nil || c.Request.Method != http.MethodGet {
		return false
	}
	return c.Request.URL.Path == "/v1/realtime" || c.Request.URL.Path == "/realtime"
}
