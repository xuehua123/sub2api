package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"
)

const (
	responsesErrorDrainTimeout  = 5 * time.Second
	responsesErrorDrainHeadroom = int64(1 << 20)
)

type responsesRawBodyContextKey struct{}

// Keep the original wire body: middleware may replace Request.Body after reading it,
// and MaxBytesReader retains its limit error instead of allowing the rest to be read.
type responsesRawBody struct {
	io.ReadCloser
	readBytes int64
	eof       bool
	writer    http.ResponseWriter
}

func (b *responsesRawBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.readBytes += int64(n)
	if errors.Is(err, io.EOF) {
		b.eof = true
	}
	return n, err
}

// Capture outside MaxBytesHandler, but wrap the response inside it. The global
// limiter must receive Go's original writer to retain its requestTooLarge hook.
func provideResponsesIngress(next http.Handler, maxBody int64, timeout time.Duration) http.Handler {
	inner := responsesErrorCommitHandler(next, maxBody, timeout)
	if maxBody > 0 {
		inner = http.MaxBytesHandler(inner, maxBody)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isResponsesErrorDrainRequest(r) {
			inner.ServeHTTP(w, r)
			return
		}
		body := &responsesRawBody{ReadCloser: r.Body, writer: w}
		// Keep the original request's Trailer map. Clone deep-copies it before
		// the body is consumed, so chunked trailers parsed during the drain would
		// otherwise be invisible to downstream handlers.
		request := r.WithContext(context.WithValue(r.Context(), responsesRawBodyContextKey{}, body))
		request.Body = body
		inner.ServeHTTP(w, request)
	})
}

func isResponsesErrorDrainRequest(r *http.Request) bool {
	if r.ProtoMajor != 1 || r.Method != http.MethodPost || r.Body == nil || r.Body == http.NoBody ||
		r.Header.Get("Upgrade") != "" {
		return false
	}
	switch r.URL.Path {
	case "/responses", "/v1/responses", "/openai/v1/responses", "/backend-api/codex/responses":
		return true
	default:
		return false
	}
}

func responsesErrorCommitHandler(next http.Handler, maxBody int64, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := r.Context().Value(responsesRawBodyContextKey{}).(*responsesRawBody)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		committed := false
		commit := func(status int) {
			if committed || (status >= 100 && status < 200 && status != http.StatusSwitchingProtocols) {
				return
			}
			committed = true
			if status >= 400 && !strings.HasPrefix(strings.ToLower(w.Header().Get("Content-Type")), "text/event-stream") {
				drainResponsesErrorBody(body, r, maxBody, timeout)
			}
		}
		// httpsnoop preserves the underlying optional interfaces and exposes Unwrap.
		// Writes, explicit Flush and ReaderFrom all pass through the commit gate.
		wrapped := httpsnoop.Wrap(w, httpsnoop.Hooks{
			WriteHeader: func(write httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
				return func(status int) { commit(status); write(status) }
			},
			Write: func(write httpsnoop.WriteFunc) httpsnoop.WriteFunc {
				return func(p []byte) (int, error) { commit(http.StatusOK); return write(p) }
			},
			Flush: func(flush httpsnoop.FlushFunc) httpsnoop.FlushFunc {
				return func() { commit(http.StatusOK); flush() }
			},
			ReadFrom: func(read httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
				return func(r io.Reader) (int64, error) { commit(http.StatusOK); return read(r) }
			},
		})
		next.ServeHTTP(wrapped, r)
	})
}

func drainResponsesErrorBody(body *responsesRawBody, r *http.Request, maxBody int64, timeout time.Duration) {
	if body.eof {
		return
	}
	started := time.Now()
	before := body.readBytes
	result := "read_error"
	controller := http.NewResponseController(body.writer)
	// Failure must remain bounded during net/http's final automatic Body.Close too.
	// A context timeout alone cannot interrupt the request's socket read.
	closeUnread := func() {
		body.writer.Header().Set("Connection", "close")
		_ = controller.SetReadDeadline(time.Now())
	}
	defer func() {
		slog.Info("responses ingress error body drain", "result", result,
			"drained_bytes", body.readBytes-before, "raw_bytes", body.readBytes,
			"duration_ms", time.Since(started).Milliseconds())
	}()
	// Reading an untouched Expect body would emit 100 Continue and invite an
	// unauthorized client to upload. Send its final error immediately instead.
	if requestExpectsContinue(r) {
		result = "expect_continue_skipped"
		closeUnread()
		return
	}
	if maxBody <= 0 || timeout <= 0 {
		result = "disabled"
		closeUnread()
		return
	}
	if err := controller.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		result = "deadline_unavailable"
		closeUnread()
		return
	}
	budget := maxBody
	if budget <= math.MaxInt64-responsesErrorDrainHeadroom {
		budget += responsesErrorDrainHeadroom
	}
	buffer := make([]byte, 32*1024)
	for !body.eof && body.readBytes < budget {
		remaining := budget - body.readBytes
		chunk := buffer
		if remaining < int64(len(chunk)) {
			chunk = chunk[:int(remaining)]
		}
		n, err := body.Read(chunk)
		if body.eof {
			result = "eof"
			if err := controller.SetReadDeadline(time.Time{}); err != nil {
				result = "deadline_reset_failed"
				closeUnread()
			}
			return
		}
		if err != nil || n == 0 {
			if err != nil {
				var timeoutErr interface{ Timeout() bool }
				if errors.As(err, &timeoutErr) && timeoutErr.Timeout() {
					result = "timeout"
				}
			}
			closeUnread()
			return
		}
	}
	result = "budget_exhausted"
	closeUnread()
}

func requestExpectsContinue(r *http.Request) bool {
	for _, value := range r.Header.Values("Expect") {
		for _, token := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "100-continue") {
				return true
			}
		}
	}
	return false
}
