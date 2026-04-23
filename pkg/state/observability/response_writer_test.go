package observability

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

const (
	testFlushMask = 1 << iota
	testHijackMask
	testPushMask
	testReadFromMask
)

func TestNewObservabilityStatusWriterSupportsAllOptionalInterfaceCombinations(t *testing.T) {
	for mask := 0; mask < 16; mask++ {
		t.Run(combinationName(mask), func(t *testing.T) {
			base, raw := newResponseWriterForMask(mask)
			wrapped := newObservabilityStatusWriter(raw)

			assertOptionalInterfaces(t, wrapped, mask)

			unwrapper, ok := wrapped.(interface{ Unwrap() http.ResponseWriter })
			if !ok {
				t.Fatal("expected wrapped writer to implement Unwrap")
			}
			if got := unwrapper.Unwrap(); got != raw {
				t.Fatalf("expected wrapped writer to unwrap original response writer, got %T", got)
			}

			if got := wrapped.StatusCode(); got != http.StatusOK {
				t.Fatalf("expected initial status code 200, got %d", got)
			}

			wrapped.WriteHeader(http.StatusCreated)
			wrapped.WriteHeader(http.StatusNoContent)
			if got := wrapped.StatusCode(); got != http.StatusCreated {
				t.Fatalf("expected status code to remain at first write header, got %d", got)
			}
			if len(base.statusCodes) != 1 || base.statusCodes[0] != http.StatusCreated {
				t.Fatalf("expected only first WriteHeader to reach wrapped writer, got %#v", base.statusCodes)
			}

			if _, err := wrapped.Write([]byte("body")); err != nil {
				t.Fatalf("Write returned error: %v", err)
			}

			if mask&testFlushMask != 0 {
				wrapped.(http.Flusher).Flush()
				if base.flushes != 1 {
					t.Fatalf("expected flush to be delegated once, got %d", base.flushes)
				}
			}

			if mask&testHijackMask != 0 {
				conn, _, err := wrapped.(http.Hijacker).Hijack()
				if err != nil {
					t.Fatalf("Hijack returned error: %v", err)
				}
				_ = conn.Close()
				if base.hijacks != 1 {
					t.Fatalf("expected hijack to be delegated once, got %d", base.hijacks)
				}
			}

			if mask&testPushMask != 0 {
				if err := wrapped.(http.Pusher).Push("/asset", nil); err != nil {
					t.Fatalf("Push returned error: %v", err)
				}
				if len(base.pushTargets) != 1 || base.pushTargets[0] != "/asset" {
					t.Fatalf("expected push target to be delegated, got %#v", base.pushTargets)
				}
			}

			if mask&testReadFromMask != 0 {
				if _, err := wrapped.(io.ReaderFrom).ReadFrom(strings.NewReader("-rf")); err != nil {
					t.Fatalf("ReadFrom returned error: %v", err)
				}
				if base.readFromCount != 1 {
					t.Fatalf("expected ReadFrom to be delegated once, got %d", base.readFromCount)
				}
			}

			if got := base.body.String(); got != "body-rf" && got != "body" {
				t.Fatalf("unexpected body contents %q", got)
			}
		})
	}
}

func TestObservabilityStatusWriterWriteKeepsDefaultStatusCode(t *testing.T) {
	base := &responseWriterBase{header: make(http.Header)}
	wrapped := newObservabilityStatusWriter(base)

	if _, err := wrapped.Write([]byte("payload")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	if got := wrapped.StatusCode(); got != http.StatusOK {
		t.Fatalf("expected default status code 200 after Write, got %d", got)
	}
	if len(base.statusCodes) != 0 {
		t.Fatalf("expected wrapped writer to avoid explicit WriteHeader on plain Write, got %#v", base.statusCodes)
	}
}

func TestObservabilityStatusWriterReadFromWithoutPriorHeader(t *testing.T) {
	base, raw := newResponseWriterForMask(testReadFromMask)
	wrapped := newObservabilityStatusWriter(raw)

	readerFrom, ok := wrapped.(io.ReaderFrom)
	if !ok {
		t.Fatal("expected wrapped writer to implement io.ReaderFrom")
	}
	if _, err := readerFrom.ReadFrom(strings.NewReader("payload")); err != nil {
		t.Fatalf("ReadFrom returned error: %v", err)
	}
	if got := wrapped.StatusCode(); got != http.StatusOK {
		t.Fatalf("expected status code 200 after ReadFrom, got %d", got)
	}
	if base.readFromCount != 1 {
		t.Fatalf("expected delegated ReadFrom count 1, got %d", base.readFromCount)
	}
}

func assertOptionalInterfaces(t *testing.T, wrapped statusResponseWriter, mask int) {
	t.Helper()

	if _, ok := wrapped.(http.Flusher); ok != (mask&testFlushMask != 0) {
		t.Fatalf("unexpected http.Flusher support for mask %s", combinationName(mask))
	}
	if _, ok := wrapped.(http.Hijacker); ok != (mask&testHijackMask != 0) {
		t.Fatalf("unexpected http.Hijacker support for mask %s", combinationName(mask))
	}
	if _, ok := wrapped.(http.Pusher); ok != (mask&testPushMask != 0) {
		t.Fatalf("unexpected http.Pusher support for mask %s", combinationName(mask))
	}
	if _, ok := wrapped.(io.ReaderFrom); ok != (mask&testReadFromMask != 0) {
		t.Fatalf("unexpected io.ReaderFrom support for mask %s", combinationName(mask))
	}
}

func combinationName(mask int) string {
	var parts []string
	if mask&testFlushMask != 0 {
		parts = append(parts, "flush")
	}
	if mask&testHijackMask != 0 {
		parts = append(parts, "hijack")
	}
	if mask&testPushMask != 0 {
		parts = append(parts, "push")
	}
	if mask&testReadFromMask != 0 {
		parts = append(parts, "readfrom")
	}
	if len(parts) == 0 {
		return "plain"
	}
	return strings.Join(parts, "_")
}

func newResponseWriterForMask(mask int) (*responseWriterBase, http.ResponseWriter) {
	base := &responseWriterBase{header: make(http.Header)}

	switch mask {
	case testFlushMask | testHijackMask | testPushMask | testReadFromMask:
		return base, struct {
			*responseWriterBase
			testFlusher
			testHijacker
			testPusher
			testReaderFrom
		}{base, testFlusher{base}, testHijacker{base}, testPusher{base}, testReaderFrom{base}}
	case testFlushMask | testHijackMask | testPushMask:
		return base, struct {
			*responseWriterBase
			testFlusher
			testHijacker
			testPusher
		}{base, testFlusher{base}, testHijacker{base}, testPusher{base}}
	case testFlushMask | testHijackMask | testReadFromMask:
		return base, struct {
			*responseWriterBase
			testFlusher
			testHijacker
			testReaderFrom
		}{base, testFlusher{base}, testHijacker{base}, testReaderFrom{base}}
	case testFlushMask | testPushMask | testReadFromMask:
		return base, struct {
			*responseWriterBase
			testFlusher
			testPusher
			testReaderFrom
		}{base, testFlusher{base}, testPusher{base}, testReaderFrom{base}}
	case testHijackMask | testPushMask | testReadFromMask:
		return base, struct {
			*responseWriterBase
			testHijacker
			testPusher
			testReaderFrom
		}{base, testHijacker{base}, testPusher{base}, testReaderFrom{base}}
	case testFlushMask | testHijackMask:
		return base, struct {
			*responseWriterBase
			testFlusher
			testHijacker
		}{base, testFlusher{base}, testHijacker{base}}
	case testFlushMask | testPushMask:
		return base, struct {
			*responseWriterBase
			testFlusher
			testPusher
		}{base, testFlusher{base}, testPusher{base}}
	case testFlushMask | testReadFromMask:
		return base, struct {
			*responseWriterBase
			testFlusher
			testReaderFrom
		}{base, testFlusher{base}, testReaderFrom{base}}
	case testHijackMask | testPushMask:
		return base, struct {
			*responseWriterBase
			testHijacker
			testPusher
		}{base, testHijacker{base}, testPusher{base}}
	case testHijackMask | testReadFromMask:
		return base, struct {
			*responseWriterBase
			testHijacker
			testReaderFrom
		}{base, testHijacker{base}, testReaderFrom{base}}
	case testPushMask | testReadFromMask:
		return base, struct {
			*responseWriterBase
			testPusher
			testReaderFrom
		}{base, testPusher{base}, testReaderFrom{base}}
	case testFlushMask:
		return base, struct {
			*responseWriterBase
			testFlusher
		}{base, testFlusher{base}}
	case testHijackMask:
		return base, struct {
			*responseWriterBase
			testHijacker
		}{base, testHijacker{base}}
	case testPushMask:
		return base, struct {
			*responseWriterBase
			testPusher
		}{base, testPusher{base}}
	case testReadFromMask:
		return base, struct {
			*responseWriterBase
			testReaderFrom
		}{base, testReaderFrom{base}}
	default:
		return base, base
	}
}

type responseWriterBase struct {
	header        http.Header
	body          bytes.Buffer
	statusCodes   []int
	flushes       int
	hijacks       int
	pushTargets   []string
	readFromCount int
}

func (w *responseWriterBase) Header() http.Header {
	return w.header
}

func (w *responseWriterBase) Write(p []byte) (int, error) {
	return w.body.Write(p)
}

func (w *responseWriterBase) WriteHeader(statusCode int) {
	w.statusCodes = append(w.statusCodes, statusCode)
}

type testFlusher struct{ base *responseWriterBase }

func (f testFlusher) Flush() {
	f.base.flushes++
}

type testHijacker struct{ base *responseWriterBase }

func (h testHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.base.hijacks++
	server, client := net.Pipe()
	_ = client.Close()
	return server, bufio.NewReadWriter(bufio.NewReader(strings.NewReader("")), bufio.NewWriter(io.Discard)), nil
}

type testPusher struct{ base *responseWriterBase }

func (p testPusher) Push(target string, opts *http.PushOptions) error {
	_ = opts
	p.base.pushTargets = append(p.base.pushTargets, target)
	return nil
}

type testReaderFrom struct{ base *responseWriterBase }

func (rf testReaderFrom) ReadFrom(r io.Reader) (int64, error) {
	rf.base.readFromCount++
	return io.Copy(&rf.base.body, r)
}
