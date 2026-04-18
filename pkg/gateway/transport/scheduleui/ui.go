package scheduleui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	runtimeschedule "github.com/1024XEngineer/anyclaw/pkg/runtime/execution/schedule"
)

// RegisterUIHandler registers the cron web UI and API handlers on the given mux.
func RegisterUIHandler(mux *http.ServeMux, scheduler *runtimeschedule.Scheduler, prefix string) {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Query().Get("json") == "1" {
				serveCronJSON(w, scheduler)
			} else {
				serveCronUI(w, scheduler)
			}
		case http.MethodPost:
			handleCreateTask(w, r, scheduler)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(prefix+"stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(scheduler.Stats())
	})

	mux.HandleFunc(prefix+"validate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		expr := r.URL.Query().Get("expr")
		desc, err := runtimeschedule.ValidateCronExpression(expr)
		resp := map[string]any{"valid": err == nil}
		if err != nil {
			resp["error"] = err.Error()
		} else {
			resp["description"] = desc
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc(prefix+"next", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		expr := r.URL.Query().Get("expr")
		count := 5
		times, err := runtimeschedule.NextRunTimes(expr, time.Now(), count)
		resp := map[string]any{}
		if err != nil {
			resp["error"] = err.Error()
		} else {
			resp["next_runs"] = times
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
}

func serveCronUI(w http.ResponseWriter, s *runtimeschedule.Scheduler) {
	tasks := s.ListTasks()
	stats := s.Stats()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Cron Task Manager</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: #f5f5f5; color: #333; }
.container { max-width: 1200px; margin: 0 auto; padding: 20px; }
h1 { margin-bottom: 20px; color: #1a1a2e; }
.stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 15px; margin-bottom: 30px; }
.stat-card { background: white; border-radius: 8px; padding: 20px; text-align: center; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
.stat-card .value { font-size: 2em; font-weight: bold; color: #4A90D9; }
.stat-card .label { color: #666; margin-top: 5px; }
.task-list { background: white; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); overflow: hidden; }
.task-header { display: grid; grid-template-columns: 2fr 1.5fr 1fr 1fr 1fr 1fr 120px; padding: 12px 20px; background: #f8f9fa; font-weight: bold; border-bottom: 1px solid #eee; }
.task-row { display: grid; grid-template-columns: 2fr 1.5fr 1fr 1fr 1fr 1fr 120px; padding: 12px 20px; border-bottom: 1px solid #f0f0f0; align-items: center; }
.task-row:hover { background: #f8f9fa; }
.task-row:last-child { border-bottom: none; }
.badge { display: inline-block; padding: 2px 8px; border-radius: 12px; font-size: 0.85em; }
.badge-enabled { background: #d4edda; color: #155724; }
.badge-disabled { background: #f8d7da; color: #721c24; }
.badge-success { background: #d4edda; color: #155724; }
.badge-failed { background: #f8d7da; color: #721c24; }
.btn { padding: 4px 12px; border: none; border-radius: 4px; cursor: pointer; font-size: 0.85em; margin-right: 4px; }
.btn-primary { background: #4A90D9; color: white; }
.btn-danger { background: #dc3545; color: white; }
.btn-success { background: #28a745; color: white; }
.btn-sm { padding: 2px 8px; font-size: 0.8em; }
.empty { padding: 40px; text-align: center; color: #999; }
</style>
</head>
<body>
<div class="container">
<h1>⏰ Cron Task Manager</h1>

<div class="stats">
<div class="stat-card"><div class="value">%d</div><div class="label">Total Tasks</div></div>
<div class="stat-card"><div class="value">%d</div><div class="label">Enabled</div></div>
<div class="stat-card"><div class="value">%d</div><div class="label">Total Runs</div></div>
<div class="stat-card"><div class="value">%d</div><div class="label">Success</div></div>
<div class="stat-card"><div class="value">%d</div><div class="label">Failed</div></div>
</div>

<div class="task-list">
<div class="task-header">
<span>Name</span><span>Schedule</span><span>Agent</span><span>Status</span><span>Last Run</span><span>Next Run</span><span>Actions</span>
</div>
`, stats["total_tasks"], stats["enabled_tasks"], stats["total_runs"], stats["success_runs"], stats["failed_runs"])

	if len(tasks) == 0 {
		fmt.Fprintf(w, `<div class="empty">No cron tasks configured. Use the API to add tasks.</div>`)
	} else {
		for _, t := range tasks {
			statusClass := "badge-enabled"
			statusText := "Enabled"
			if !t.Enabled {
				statusClass = "badge-disabled"
				statusText = "Disabled"
			}
			lastRun := "-"
			if t.LastRun != nil {
				lastRun = t.LastRun.Format("2006-01-02 15:04")
			}
			nextRun := "-"
			if t.NextRun != nil {
				nextRun = t.NextRun.Format("2006-01-02 15:04")
			}
			agent := t.Agent
			if agent == "" {
				agent = "-"
			}
			fmt.Fprintf(w, `<div class="task-row">
<span><strong>%s</strong><br><small style="color:#999">%s</small></span>
<span><code>%s</code></span>
<span>%s</span>
<span><span class="badge %s">%s</span></span>
<span>%s</span>
<span>%s</span>
<span>
<button class="btn btn-primary btn-sm" onclick="runTask('%s')">Run</button>
<button class="btn btn-sm" style="background:#6c757d;color:white" onclick="toggleTask('%s', %t)">Toggle</button>
</span>
</div>`, t.Name, t.Command, t.Schedule, agent, statusClass, statusText, lastRun, nextRun, t.ID, t.ID, t.Enabled)
		}
	}

	fmt.Fprintf(w, `</div>
</div>
<script>
function runTask(id) { fetch('/cron/'+id+'/run', {method:'POST'}).then(r=>r.json()).then(d=>alert(d.message||JSON.stringify(d))); }
function toggleTask(id, enabled) { fetch('/cron/'+id+(enabled?'/disable':'/enable'), {method:'POST'}).then(r=>r.json()).then(d=>location.reload()); }
</script>
</body>
</html>`)
}

func serveCronJSON(w http.ResponseWriter, s *runtimeschedule.Scheduler) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"tasks": s.ListTasks(),
		"stats": s.Stats(),
	})
}

func handleCreateTask(w http.ResponseWriter, r *http.Request, s *runtimeschedule.Scheduler) {
	var task runtimeschedule.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id, err := s.AddTask(&task)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "message": "Task created"})
}
