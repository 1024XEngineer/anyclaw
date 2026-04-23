package observability

import (
	"net/http"
	"net/http/pprof"
	"strings"
)

// PprofHandler returns an http.Handler that serves pprof endpoints.
// It serves the standard pprof endpoints under /debug/pprof/.
func PprofHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	return mux
}

// RegisterPprof registers pprof handlers on an existing ServeMux.
func RegisterPprof(mux *http.ServeMux, prefix string) {
	prefix = normalizePprofPrefix(prefix)
	mux.HandleFunc(prefix, pprof.Index)
	mux.HandleFunc(prefix+"cmdline", pprof.Cmdline)
	mux.HandleFunc(prefix+"profile", pprof.Profile)
	mux.HandleFunc(prefix+"symbol", pprof.Symbol)
	mux.HandleFunc(prefix+"trace", pprof.Trace)
	mux.Handle(prefix+"allocs", pprof.Handler("allocs"))
	mux.Handle(prefix+"block", pprof.Handler("block"))
	mux.Handle(prefix+"goroutine", pprof.Handler("goroutine"))
	mux.Handle(prefix+"heap", pprof.Handler("heap"))
	mux.Handle(prefix+"mutex", pprof.Handler("mutex"))
	mux.Handle(prefix+"threadcreate", pprof.Handler("threadcreate"))
}

func normalizePprofPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "/debug/pprof/"
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}
