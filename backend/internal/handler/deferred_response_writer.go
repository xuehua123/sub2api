package handler

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const deferredUsageRecordTimeout = 10 * time.Second

type deferredGatewayResponse struct {
	c        *gin.Context
	original gin.ResponseWriter
	writer   *deferredResponseWriter
	done     bool
}

func beginDeferredGatewayResponse(c *gin.Context, enabled bool) *deferredGatewayResponse {
	if !enabled || c == nil || c.Writer == nil || c.Writer.Written() {
		return nil
	}
	original := c.Writer
	writer := newDeferredResponseWriter(original)
	c.Writer = writer
	return &deferredGatewayResponse{
		c:        c,
		original: original,
		writer:   writer,
	}
}

func (d *deferredGatewayResponse) Flush() {
	if d == nil || d.done {
		return
	}
	d.done = true
	if d.c != nil && d.c.Writer == d.writer {
		d.c.Writer = d.original
	}
	d.writer.FlushTo(d.original)
}

func (d *deferredGatewayResponse) Discard() {
	if d == nil || d.done {
		return
	}
	d.done = true
	if d.c != nil && d.c.Writer == d.writer {
		d.c.Writer = d.original
	}
}

func (d *deferredGatewayResponse) Reset() {
	if d == nil || d.done || d.writer == nil {
		return
	}
	d.writer.Reset()
}

type deferredResponseWriter struct {
	original gin.ResponseWriter
	header   http.Header
	body     bytes.Buffer
	status   int
	size     int
}

func newDeferredResponseWriter(original gin.ResponseWriter) *deferredResponseWriter {
	w := &deferredResponseWriter{original: original}
	w.header = cloneHeader(original.Header())
	w.Reset()
	return w
}

func (w *deferredResponseWriter) Reset() {
	w.body.Reset()
	w.status = http.StatusOK
	w.size = -1
	if w.header == nil {
		w.header = make(http.Header)
	}
}

func (w *deferredResponseWriter) Header() http.Header {
	return w.header
}

func (w *deferredResponseWriter) WriteHeader(code int) {
	if code <= 0 || w.Written() {
		return
	}
	w.status = code
}

func (w *deferredResponseWriter) WriteHeaderNow() {
	if !w.Written() {
		w.size = 0
	}
}

func (w *deferredResponseWriter) Write(data []byte) (int, error) {
	w.WriteHeaderNow()
	n, err := w.body.Write(data)
	w.size += n
	return n, err
}

func (w *deferredResponseWriter) WriteString(s string) (int, error) {
	w.WriteHeaderNow()
	n, err := w.body.WriteString(s)
	w.size += n
	return n, err
}

func (w *deferredResponseWriter) Status() int {
	return w.status
}

func (w *deferredResponseWriter) Size() int {
	return w.size
}

func (w *deferredResponseWriter) Written() bool {
	return w.size >= 0
}

func (w *deferredResponseWriter) Flush() {
	w.WriteHeaderNow()
}

func (w *deferredResponseWriter) Pusher() http.Pusher {
	return w.original.Pusher()
}

func (w *deferredResponseWriter) CloseNotify() <-chan bool {
	return w.original.CloseNotify()
}

func (w *deferredResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.original.Hijack()
}

func (w *deferredResponseWriter) FlushTo(dst gin.ResponseWriter) {
	if dst == nil {
		return
	}
	copyHeader(dst.Header(), w.header)
	if !w.Written() {
		return
	}
	dst.WriteHeader(w.status)
	if w.body.Len() > 0 {
		_, _ = dst.Write(w.body.Bytes())
	}
}

func cloneHeader(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	copyHeader(dst, src)
	return dst
}

func copyHeader(dst, src http.Header) {
	if dst == nil || src == nil {
		return
	}
	for key := range src {
		dst.Del(key)
	}
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func runUsageRecordTaskSync(parent context.Context, task func(context.Context) error) (err error) {
	if task == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(usageRecordContext(parent, context.Background()), deferredUsageRecordTimeout)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("usage record task panic: %v", recovered)
		}
	}()
	return task(ctx)
}

func gatewayUsageRecordTask(
	gatewayService *service.GatewayService,
	input *service.RecordUsageInput,
) func(context.Context) error {
	return func(ctx context.Context) error {
		return gatewayService.RecordUsage(ctx, input)
	}
}
