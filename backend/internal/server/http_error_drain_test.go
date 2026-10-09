//go:build unit

package server

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
)

const responsesIngressErrorPayload = `{"error":{"message":"` + "early rejection remains an HTTP response" + `"}}`

func responsesIngressLargeErrorPayload() string {
	return fmt.Sprintf(`{"error":{"message":%q}}`, strings.Repeat("rejected ", 640))
}

func responsesIngressTCPServer(t *testing.T, handler http.Handler, maxBody int64, timeout time.Duration) string {
	t.Helper()
	srv := &http.Server{
		Handler:           provideResponsesIngress(handler, maxBody, timeout),
		ReadHeaderTimeout: time.Second,
		IdleTimeout:       2 * time.Second,
	}
	addr, stop := serveIngressTestServer(t, srv)
	t.Cleanup(stop)
	return addr
}

func responsesIngressDial(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
	return conn
}

func responsesIngressReadResponse(t *testing.T, reader *bufio.Reader) *http.Response {
	t.Helper()
	response, err := http.ReadResponse(reader, nil)
	require.NoError(t, err)
	return response
}

func responsesIngressReadPayload(t *testing.T, response *http.Response) string {
	t.Helper()
	payload, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	return string(payload)
}

func responsesIngressUpload(t *testing.T, conn net.Conn, body string) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		n, err := io.WriteString(conn, body)
		if err == nil && n != len(body) {
			err = io.ErrShortWrite
		}
		done <- err
	}()
	return done
}

// A request rejected before reading its 512 KiB body must still produce the
// complete error and leave an ordinary keep-alive connection usable. The old
// net/http 256 KiB automatic drain closes that connection instead.
func TestResponsesIngressEarlyErrorsDrainBeforeTCPCommit(t *testing.T) {
	cases := []struct {
		path   string
		status int
		flush  bool
	}{
		{path: "/responses", status: http.StatusUnauthorized},
		{path: "/v1/responses", status: http.StatusTooManyRequests},
		{path: "/openai/v1/responses", status: http.StatusUnauthorized},
		{path: "/backend-api/codex/responses", status: http.StatusTooManyRequests, flush: true},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			payload := responsesIngressLargeErrorPayload()
			addr := responsesIngressTCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					_, _ = io.WriteString(w, "next request")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if tc.flush {
					w.(http.Flusher).Flush()
				}
				_, _ = io.WriteString(w, payload)
			}), 1<<20, time.Second)
			conn := responsesIngressDial(t, addr)
			body := strings.Repeat("x", 512<<10)
			_, err := fmt.Fprintf(conn, "POST %s HTTP/1.1\r\nHost: test\r\nContent-Length: %d\r\n\r\n", tc.path, len(body))
			require.NoError(t, err)
			upload := responsesIngressUpload(t, conn, body)
			reader := bufio.NewReader(conn)
			response := responsesIngressReadResponse(t, reader)
			require.Equal(t, tc.status, response.StatusCode)
			require.Equal(t, payload, responsesIngressReadPayload(t, response))
			require.NoError(t, <-upload)
			require.False(t, response.Close, "fully drained early errors retain ordinary keep-alive")
			_, err = io.WriteString(conn, "GET /next HTTP/1.1\r\nHost: test\r\n\r\n")
			require.NoError(t, err)
			next := responsesIngressReadResponse(t, reader)
			require.Equal(t, http.StatusOK, next.StatusCode)
			require.Equal(t, "next request", responsesIngressReadPayload(t, next))
		})
	}
}

func TestResponsesIngressChunkedErrorDrainsThroughTrailers(t *testing.T) {
	payload := responsesIngressLargeErrorPayload()
	trailerSeen := make(chan string, 1)
	addr := responsesIngressTCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, "next request")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, payload)
		trailerSeen <- r.Trailer.Get("X-Upload-End")
	}), 1<<20, time.Second)
	conn := responsesIngressDial(t, addr)
	_, err := io.WriteString(conn, "POST /responses HTTP/1.1\r\nHost: test\r\nTransfer-Encoding: chunked\r\nTrailer: X-Upload-End\r\n\r\n")
	require.NoError(t, err)
	var chunks strings.Builder
	for range 16 {
		_, _ = fmt.Fprintf(&chunks, "%x\r\n%s\r\n", 32<<10, strings.Repeat("x", 32<<10))
	}
	chunks.WriteString("0\r\nX-Upload-End: complete\r\n\r\n")
	upload := responsesIngressUpload(t, conn, chunks.String())
	reader := bufio.NewReader(conn)
	response := responsesIngressReadResponse(t, reader)
	require.Equal(t, http.StatusTooManyRequests, response.StatusCode)
	require.Equal(t, payload, responsesIngressReadPayload(t, response))
	require.NoError(t, <-upload)
	require.Equal(t, "complete", <-trailerSeen)
	require.False(t, response.Close)
	_, err = io.WriteString(conn, "GET /next HTTP/1.1\r\nHost: test\r\n\r\n")
	require.NoError(t, err)
	next := responsesIngressReadResponse(t, reader)
	require.Equal(t, "next request", responsesIngressReadPayload(t, next))
}

func TestResponsesIngressGinCommitThroughProvideHTTPServer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name   string
		status int
		flush  bool
	}{
		{name: "JSON401", status: http.StatusUnauthorized},
		{name: "JSON429", status: http.StatusTooManyRequests},
		{name: "status flush", status: http.StatusTooManyRequests, flush: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			message := strings.Repeat("rejected ", 640)
			payload := fmt.Sprintf(`{"error":{"message":%q}}`, message)
			router := gin.New()
			router.POST("/responses", func(c *gin.Context) {
				if tc.flush {
					c.Header("Content-Type", "application/json")
					c.Status(tc.status)
					c.Writer.Flush()
					_, _ = c.Writer.WriteString(payload)
					return
				}
				c.JSON(tc.status, gin.H{"error": gin.H{"message": message}})
			})
			router.GET("/next", func(c *gin.Context) { c.String(http.StatusOK, "next request") })
			cfg := ingressTestConfig()
			cfg.Server.MaxRequestBodySize = 1 << 20
			cfg.Gateway.MaxBodySize = 1 << 20
			addr, stop := serveIngressTestServer(t, ProvideHTTPServer(cfg, router))
			t.Cleanup(stop)
			conn := responsesIngressDial(t, addr)
			body := strings.Repeat("x", 512<<10)
			_, err := fmt.Fprintf(conn, "POST /responses HTTP/1.1\r\nHost: test\r\nContent-Length: %d\r\n\r\n", len(body))
			require.NoError(t, err)
			upload := responsesIngressUpload(t, conn, body)
			reader := bufio.NewReader(conn)
			response := responsesIngressReadResponse(t, reader)
			require.Equal(t, tc.status, response.StatusCode)
			require.Equal(t, payload, responsesIngressReadPayload(t, response))
			require.NoError(t, <-upload)
			require.False(t, response.Close)
			_, err = io.WriteString(conn, "GET /next HTTP/1.1\r\nHost: test\r\n\r\n")
			require.NoError(t, err)
			next := responsesIngressReadResponse(t, reader)
			require.Equal(t, "next request", responsesIngressReadPayload(t, next))
		})
	}
}

func TestResponsesIngressBodyLimitRetains413AndGoConnectionClose(t *testing.T) {
	readFailure := make(chan error, 1)
	payload := responsesIngressLargeErrorPayload()
	const limit = int64(256 << 10)
	addr := responsesIngressTCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		readFailure <- err
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = io.WriteString(w, payload)
	}), limit, time.Second)
	conn := responsesIngressDial(t, addr)
	body := strings.Repeat("x", 512<<10)
	_, err := fmt.Fprintf(conn, "POST /responses HTTP/1.1\r\nHost: test\r\nContent-Length: %d\r\n\r\n", len(body))
	require.NoError(t, err)
	upload := responsesIngressUpload(t, conn, body)
	response := responsesIngressReadResponse(t, bufio.NewReader(conn))
	require.Equal(t, http.StatusRequestEntityTooLarge, response.StatusCode)
	require.Equal(t, payload, responsesIngressReadPayload(t, response))
	require.NoError(t, <-upload)
	var maxError *http.MaxBytesError
	require.ErrorAs(t, <-readFailure, &maxError)
	require.Equal(t, limit, maxError.Limit)
	require.True(t, response.Close, "the original Go writer must retain requestTooLarge semantics")
}

func TestResponsesIngressTCPBudgetExhaustionDoesNotWaitForRemainingBody(t *testing.T) {
	const limit = int64(64 << 10)
	const totalBudget = limit + (1 << 20)
	readFailure := make(chan error, 1)
	payload := responsesIngressLargeErrorPayload()
	addr := responsesIngressTCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		readFailure <- err
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = io.WriteString(w, payload)
	}), limit, time.Second)
	conn := responsesIngressDial(t, addr)
	// Leave one declared byte unsent after exactly the total drain budget.
	// net/http's final Body.Close would try to read that byte if its deadline
	// were reset after mistaking budget exhaustion for a real EOF.
	_, err := fmt.Fprintf(conn, "POST /responses HTTP/1.1\r\nHost: test\r\nContent-Length: %d\r\n\r\n", totalBudget+1)
	require.NoError(t, err)
	upload := responsesIngressUpload(t, conn, strings.Repeat("x", int(totalBudget)))
	started := time.Now()
	reader := bufio.NewReader(conn)
	response := responsesIngressReadResponse(t, reader)
	require.Equal(t, http.StatusRequestEntityTooLarge, response.StatusCode)
	require.Equal(t, payload, responsesIngressReadPayload(t, response))
	require.True(t, response.Close)
	require.NoError(t, <-upload)
	require.Less(t, time.Since(started), 1500*time.Millisecond)
	var maxError *http.MaxBytesError
	require.ErrorAs(t, <-readFailure, &maxError)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	_, err = reader.ReadByte()
	require.Error(t, err)
	var timeoutError net.Error
	require.False(t, errors.As(err, &timeoutError) && timeoutError.Timeout())
}

func TestResponsesIngressInformationalResponseDoesNotCommitDrainGate(t *testing.T) {
	addr := responsesIngressTCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusEarlyHints)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, responsesIngressErrorPayload)
	}), 1<<20, time.Second)
	conn := responsesIngressDial(t, addr)
	body := strings.Repeat("x", 512<<10)
	_, err := fmt.Fprintf(conn, "POST /responses HTTP/1.1\r\nHost: test\r\nContent-Length: %d\r\n\r\n", len(body))
	require.NoError(t, err)
	upload := responsesIngressUpload(t, conn, body)
	reader := bufio.NewReader(conn)
	hints := responsesIngressReadResponse(t, reader)
	require.Equal(t, http.StatusEarlyHints, hints.StatusCode)
	response := responsesIngressReadResponse(t, reader)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.Equal(t, responsesIngressErrorPayload, responsesIngressReadPayload(t, response))
	require.NoError(t, <-upload)
	require.False(t, response.Close)
}

func TestResponsesIngressSlowBodyDeadlineAlsoBoundsFinalClose(t *testing.T) {
	payload := responsesIngressLargeErrorPayload()
	addr := responsesIngressTCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, payload)
	}), 1<<20, 80*time.Millisecond)
	conn := responsesIngressDial(t, addr)
	// Less than Go's automatic-close tolerance exercises its final Body.Close.
	_, err := io.WriteString(conn, "POST /responses HTTP/1.1\r\nHost: test\r\nContent-Length: 65536\r\n\r\n")
	require.NoError(t, err)
	started := time.Now()
	reader := bufio.NewReader(conn)
	response := responsesIngressReadResponse(t, reader)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.Equal(t, payload, responsesIngressReadPayload(t, response))
	require.True(t, response.Close)
	require.Less(t, time.Since(started), 1500*time.Millisecond)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	_, err = reader.ReadByte()
	require.Error(t, err)
	var timeoutError net.Error
	require.False(t, errors.As(err, &timeoutError) && timeoutError.Timeout(), "final Body.Close must not resume an unbounded read")
}

func TestResponsesIngressExpectContinueReturnsFinalErrorWithoutInvitingUpload(t *testing.T) {
	for _, framing := range []string{"Content-Length: 65536", "Transfer-Encoding: chunked"} {
		t.Run(framing, func(t *testing.T) {
			addr := responsesIngressTCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, responsesIngressErrorPayload)
			}), 1<<20, time.Second)
			conn := responsesIngressDial(t, addr)
			_, err := fmt.Fprintf(conn, "POST /responses HTTP/1.1\r\nHost: test\r\nExpect: 100-continue\r\n%s\r\n\r\n", framing)
			require.NoError(t, err)
			reader := bufio.NewReader(conn)
			response := responsesIngressReadResponse(t, reader)
			require.Equal(t, http.StatusUnauthorized, response.StatusCode, "no intermediate 100 Continue")
			require.Equal(t, responsesIngressErrorPayload, responsesIngressReadPayload(t, response))
			require.True(t, response.Close)
			require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
			_, err = reader.ReadByte()
			require.Error(t, err)
			var timeoutError net.Error
			require.False(t, errors.As(err, &timeoutError) && timeoutError.Timeout())
		})
	}
}

type responsesIngressDeadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
	failure   error
}

func (w *responsesIngressDeadlineRecorder) SetReadDeadline(deadline time.Time) error {
	w.deadlines = append(w.deadlines, deadline)
	return w.failure
}

type responsesIngressReadSpy struct {
	reader       *strings.Reader
	readBytes    int64
	reads        int
	eofWithBytes bool
}

func (b *responsesIngressReadSpy) Read(p []byte) (int, error) {
	b.reads++
	n, err := b.reader.Read(p)
	b.readBytes += int64(n)
	if b.eofWithBytes && b.reader.Len() == 0 {
		err = io.EOF
	}
	return n, err
}

func (*responsesIngressReadSpy) Close() error { return nil }

func TestResponsesIngressTotalBudgetCountsBytesAlreadyRead(t *testing.T) {
	const limit = int64(64 << 10)
	const totalBudget = limit + (1 << 20)
	body := &responsesIngressReadSpy{reader: strings.NewReader(strings.Repeat("x", int(totalBudget+1)))}
	recorder := &responsesIngressDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler := provideResponsesIngress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		var maxError *http.MaxBytesError
		require.ErrorAs(t, err, &maxError)
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = io.WriteString(w, responsesIngressErrorPayload)
	}), limit, time.Second)
	request := httptest.NewRequest(http.MethodPost, "/responses", nil)
	request.Body = body
	request.ContentLength = totalBudget + 1
	handler.ServeHTTP(recorder, request)
	require.Equal(t, totalBudget, body.readBytes, "headroom is part of the total raw budget, not additional per-drain budget")
	require.Equal(t, "close", recorder.Header().Get("Connection"))
	require.NotEmpty(t, recorder.deadlines)
	require.False(t, recorder.deadlines[len(recorder.deadlines)-1].IsZero())
}

func TestResponsesIngressBudgetBoundaryRequiresRealEOF(t *testing.T) {
	const limit = int64(32 << 10)
	const totalBudget = limit + (1 << 20)
	for _, finalEOF := range []bool{false, true} {
		t.Run(fmt.Sprintf("EOF_with_final_bytes_%t", finalEOF), func(t *testing.T) {
			body := &responsesIngressReadSpy{
				reader:       strings.NewReader(strings.Repeat("x", int(totalBudget))),
				eofWithBytes: finalEOF,
			}
			recorder := &responsesIngressDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
			handler := provideResponsesIngress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			}), limit, time.Second)
			request := httptest.NewRequest(http.MethodPost, "/responses", nil)
			request.Body = body
			request.ContentLength = totalBudget
			handler.ServeHTTP(recorder, request)
			require.Equal(t, totalBudget, body.readBytes)
			require.NotEmpty(t, recorder.deadlines)
			if finalEOF {
				require.Empty(t, recorder.Header().Get("Connection"))
				require.True(t, recorder.deadlines[len(recorder.deadlines)-1].IsZero())
			} else {
				require.Equal(t, "close", recorder.Header().Get("Connection"))
				require.False(t, recorder.deadlines[len(recorder.deadlines)-1].IsZero())
			}
		})
	}
}

func TestResponsesIngressSkipsDrainWithoutSocketDeadline(t *testing.T) {
	body := &responsesIngressReadSpy{reader: strings.NewReader("unread body")}
	recorder := &responsesIngressDeadlineRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		failure:          http.ErrNotSupported,
	}
	handler := provideResponsesIngress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}), 1024, time.Second)
	request := httptest.NewRequest(http.MethodPost, "/responses", nil)
	request.Body = body
	handler.ServeHTTP(recorder, request)
	require.Zero(t, body.reads)
	require.Equal(t, "close", recorder.Header().Get("Connection"))
}

func TestResponsesIngressUsesOriginalBodyAfterDownstreamReplacement(t *testing.T) {
	for _, fullyRead := range []bool{false, true} {
		t.Run(fmt.Sprintf("original_fully_read_%t", fullyRead), func(t *testing.T) {
			body := &responsesIngressReadSpy{reader: strings.NewReader(strings.Repeat("x", 512<<10))}
			replacement := &responsesIngressReadSpy{reader: strings.NewReader("replacement must remain unread")}
			recorder := &responsesIngressDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
			handler := provideResponsesIngress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if fullyRead {
					_, err := io.ReadAll(r.Body)
					require.NoError(t, err)
				} else {
					_, err := io.CopyN(io.Discard, r.Body, 64<<10)
					require.NoError(t, err)
				}
				r.Body = replacement
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, responsesIngressErrorPayload)
			}), 1<<20, time.Second)
			request := httptest.NewRequest(http.MethodPost, "/responses", nil)
			request.Body = body
			request.ContentLength = 512 << 10
			handler.ServeHTTP(recorder, request)
			require.Equal(t, int64(512<<10), body.readBytes)
			require.Zero(t, replacement.reads)
			require.Empty(t, recorder.Header().Get("Connection"))
			if fullyRead {
				require.Empty(t, recorder.deadlines, "a previously observed EOF needs no drain or deadline change")
			} else {
				require.NotEmpty(t, recorder.deadlines)
				require.True(t, recorder.deadlines[len(recorder.deadlines)-1].IsZero())
			}
		})
	}
}

func TestResponsesIngressBypassesSuccessfulStreamsAndOtherProtocols(t *testing.T) {
	cases := []struct {
		name        string
		method      string
		path        string
		protoMajor  int
		upgrade     string
		contentType string
		status      int
	}{
		{name: "SSE 200", method: http.MethodPost, path: "/responses", protoMajor: 1, contentType: "text/event-stream", status: http.StatusOK},
		{name: "SSE error", method: http.MethodPost, path: "/responses", protoMajor: 1, contentType: "text/event-stream; charset=utf-8", status: http.StatusBadRequest},
		{name: "websocket GET", method: http.MethodGet, path: "/responses", protoMajor: 1, upgrade: "websocket", status: http.StatusUnauthorized},
		{name: "websocket POST", method: http.MethodPost, path: "/responses", protoMajor: 1, upgrade: "websocket", status: http.StatusUnauthorized},
		{name: "HTTP2", method: http.MethodPost, path: "/responses", protoMajor: 2, status: http.StatusUnauthorized},
		{name: "unrelated API", method: http.MethodPost, path: "/chat/completions", protoMajor: 1, status: http.StatusUnauthorized},
		{name: "responses subpath", method: http.MethodPost, path: "/responses/compact", protoMajor: 1, status: http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := &responsesIngressReadSpy{reader: strings.NewReader("unread body")}
			recorder := &responsesIngressDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
			handler := provideResponsesIngress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				w.(http.Flusher).Flush()
				_, _ = io.WriteString(w, "stream event")
			}), 1024, time.Second)
			request := httptest.NewRequest(tc.method, tc.path, nil)
			request.Body = body
			request.ProtoMajor = tc.protoMajor
			request.Header.Set("Upgrade", tc.upgrade)
			handler.ServeHTTP(recorder, request)
			require.Zero(t, body.reads)
			require.Empty(t, recorder.deadlines)
			require.Equal(t, tc.status, recorder.Code)
			require.Equal(t, "stream event", recorder.Body.String())
		})
	}
}

func TestResponsesIngressWebSocketHijackRemainsUsable(t *testing.T) {
	serverError := make(chan error, 1)
	addr := responsesIngressTCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			serverError <- err
			return
		}
		defer func() { _ = conn.Close() }()
		_, err = io.WriteString(buffered, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\nready\n")
		if err == nil {
			err = buffered.Flush()
		}
		serverError <- err
	}), 1024, time.Second)
	conn := responsesIngressDial(t, addr)
	_, err := io.WriteString(conn, "GET /responses HTTP/1.1\r\nHost: test\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
	require.NoError(t, err)
	reader := bufio.NewReader(conn)
	response := responsesIngressReadResponse(t, reader)
	require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)
	message, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "ready\n", message)
	require.NoError(t, <-serverError)
}

func TestResponsesIngressHTTP2RetainsGlobalBodyLimit(t *testing.T) {
	protocol := make(chan int, 1)
	readFailure := make(chan error, 1)
	handler := provideResponsesIngress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protocol <- r.ProtoMajor
		_, err := io.ReadAll(r.Body)
		readFailure <- err
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = io.WriteString(w, responsesIngressErrorPayload)
	}), 1024, time.Second)
	srv := httptest.NewUnstartedServer(handler)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	transport := &http2.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} // Local httptest certificate.
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	response, err := client.Post(srv.URL+"/responses", "application/json", bytes.NewBufferString(strings.Repeat("x", 1025)))
	require.NoError(t, err)
	require.Equal(t, 2, <-protocol)
	var maxError *http.MaxBytesError
	require.ErrorAs(t, <-readFailure, &maxError)
	require.Equal(t, int64(1024), maxError.Limit)
	require.Equal(t, http.StatusRequestEntityTooLarge, response.StatusCode)
	require.Equal(t, responsesIngressErrorPayload, responsesIngressReadPayload(t, response))
}
