package service

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
)

func compileResponseHeaderFilter(cfg *config.Config) *responseheaders.CompiledHeaderFilter {
	if cfg == nil {
		return nil
	}
	return responseheaders.CompileHeaderFilter(cfg.Security.ResponseHeaders)
}

func writeFilteredHeadersForTransformedJSON(dst http.Header, src http.Header, filter *responseheaders.CompiledHeaderFilter) {
	responseheaders.WriteFilteredHeaders(dst, src, filter)
	// The body is rebuilt locally, so upstream entity headers no longer
	// describe the bytes sent to the client.
	dst.Del("Content-Type")
	dst.Del("Content-Encoding")
}
