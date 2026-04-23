package observability

import (
	"bufio"
	"io"
	"net"
	"net/http"
)

type statusResponseWriter interface {
	http.ResponseWriter
	StatusCode() int
}

type observabilityStatusWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func newObservabilityStatusWriter(w http.ResponseWriter) statusResponseWriter {
	recorder := &observabilityStatusWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	_, canFlush := w.(http.Flusher)
	_, canHijack := w.(http.Hijacker)
	_, canPush := w.(http.Pusher)
	_, canReadFrom := w.(io.ReaderFrom)

	switch {
	case canFlush && canHijack && canPush && canReadFrom:
		return struct {
			*observabilityStatusWriter
			flushFeature
			hijackFeature
			pushFeature
			readerFromFeature
		}{recorder, flushFeature{recorder}, hijackFeature{recorder}, pushFeature{recorder}, readerFromFeature{recorder}}
	case canFlush && canHijack && canPush:
		return struct {
			*observabilityStatusWriter
			flushFeature
			hijackFeature
			pushFeature
		}{recorder, flushFeature{recorder}, hijackFeature{recorder}, pushFeature{recorder}}
	case canFlush && canHijack && canReadFrom:
		return struct {
			*observabilityStatusWriter
			flushFeature
			hijackFeature
			readerFromFeature
		}{recorder, flushFeature{recorder}, hijackFeature{recorder}, readerFromFeature{recorder}}
	case canFlush && canPush && canReadFrom:
		return struct {
			*observabilityStatusWriter
			flushFeature
			pushFeature
			readerFromFeature
		}{recorder, flushFeature{recorder}, pushFeature{recorder}, readerFromFeature{recorder}}
	case canHijack && canPush && canReadFrom:
		return struct {
			*observabilityStatusWriter
			hijackFeature
			pushFeature
			readerFromFeature
		}{recorder, hijackFeature{recorder}, pushFeature{recorder}, readerFromFeature{recorder}}
	case canFlush && canHijack:
		return struct {
			*observabilityStatusWriter
			flushFeature
			hijackFeature
		}{recorder, flushFeature{recorder}, hijackFeature{recorder}}
	case canFlush && canPush:
		return struct {
			*observabilityStatusWriter
			flushFeature
			pushFeature
		}{recorder, flushFeature{recorder}, pushFeature{recorder}}
	case canFlush && canReadFrom:
		return struct {
			*observabilityStatusWriter
			flushFeature
			readerFromFeature
		}{recorder, flushFeature{recorder}, readerFromFeature{recorder}}
	case canHijack && canPush:
		return struct {
			*observabilityStatusWriter
			hijackFeature
			pushFeature
		}{recorder, hijackFeature{recorder}, pushFeature{recorder}}
	case canHijack && canReadFrom:
		return struct {
			*observabilityStatusWriter
			hijackFeature
			readerFromFeature
		}{recorder, hijackFeature{recorder}, readerFromFeature{recorder}}
	case canPush && canReadFrom:
		return struct {
			*observabilityStatusWriter
			pushFeature
			readerFromFeature
		}{recorder, pushFeature{recorder}, readerFromFeature{recorder}}
	case canFlush:
		return struct {
			*observabilityStatusWriter
			flushFeature
		}{recorder, flushFeature{recorder}}
	case canHijack:
		return struct {
			*observabilityStatusWriter
			hijackFeature
		}{recorder, hijackFeature{recorder}}
	case canPush:
		return struct {
			*observabilityStatusWriter
			pushFeature
		}{recorder, pushFeature{recorder}}
	case canReadFrom:
		return struct {
			*observabilityStatusWriter
			readerFromFeature
		}{recorder, readerFromFeature{recorder}}
	default:
		return recorder
	}
}

func (sw *observabilityStatusWriter) WriteHeader(code int) {
	if sw.wroteHeader {
		return
	}
	sw.wroteHeader = true
	sw.statusCode = code
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *observabilityStatusWriter) Write(p []byte) (int, error) {
	if !sw.wroteHeader {
		sw.wroteHeader = true
	}
	return sw.ResponseWriter.Write(p)
}

func (sw *observabilityStatusWriter) StatusCode() int {
	return sw.statusCode
}

func (sw *observabilityStatusWriter) Unwrap() http.ResponseWriter {
	return sw.ResponseWriter
}

func (sw *observabilityStatusWriter) flush() {
	if !sw.wroteHeader {
		sw.wroteHeader = true
	}
	sw.ResponseWriter.(http.Flusher).Flush()
}

func (sw *observabilityStatusWriter) hijack() (net.Conn, *bufio.ReadWriter, error) {
	return sw.ResponseWriter.(http.Hijacker).Hijack()
}

func (sw *observabilityStatusWriter) push(target string, opts *http.PushOptions) error {
	return sw.ResponseWriter.(http.Pusher).Push(target, opts)
}

func (sw *observabilityStatusWriter) readFrom(r io.Reader) (int64, error) {
	if !sw.wroteHeader {
		sw.wroteHeader = true
	}
	return sw.ResponseWriter.(io.ReaderFrom).ReadFrom(r)
}

type flushFeature struct {
	recorder *observabilityStatusWriter
}

func (f flushFeature) Flush() {
	f.recorder.flush()
}

type hijackFeature struct {
	recorder *observabilityStatusWriter
}

func (h hijackFeature) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return h.recorder.hijack()
}

type pushFeature struct {
	recorder *observabilityStatusWriter
}

func (p pushFeature) Push(target string, opts *http.PushOptions) error {
	return p.recorder.push(target, opts)
}

type readerFromFeature struct {
	recorder *observabilityStatusWriter
}

func (rf readerFromFeature) ReadFrom(r io.Reader) (int64, error) {
	return rf.recorder.readFrom(r)
}
