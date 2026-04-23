package qmd

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": message,
		"code":  status,
	})
}

func (s *Server) handleCreateTable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string   `json:"name"`
		Columns []string `json:"columns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if err := s.store.CreateTable(req.Name, req.Columns); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "created", "table": req.Name})
}

func (s *Server) handleDropTable(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "table name is required")
		return
	}

	if err := s.store.DropTable(name); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "dropped", "table": name})
}

func (s *Server) handleListTables(w http.ResponseWriter, r *http.Request) {
	stats := s.store.Stats()
	writeJSON(w, http.StatusOK, stats.MemoryTables)
}

func (s *Server) handleGetTable(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "table name is required")
		return
	}

	stats := s.store.Stats()
	for _, t := range stats.MemoryTables {
		if t.Name == name {
			writeJSON(w, http.StatusOK, t)
			return
		}
	}

	writeError(w, http.StatusNotFound, "table not found")
}

func (s *Server) handleInsert(w http.ResponseWriter, r *http.Request) {
	table := r.PathValue("table")
	if table == "" {
		writeError(w, http.StatusBadRequest, "table is required")
		return
	}

	var req struct {
		ID   string         `json:"id"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Data == nil {
		writeError(w, http.StatusBadRequest, "data is required")
		return
	}

	record := &Record{
		ID:   req.ID,
		Data: req.Data,
	}

	if err := s.store.Insert(table, record); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	table := r.PathValue("table")
	id := r.PathValue("id")

	if table == "" || id == "" {
		writeError(w, http.StatusBadRequest, "table and id are required")
		return
	}

	record, err := s.store.Get(table, id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	table := r.PathValue("table")
	id := r.PathValue("id")

	if table == "" || id == "" {
		writeError(w, http.StatusBadRequest, "table and id are required")
		return
	}

	var req struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	record := &Record{
		ID:   id,
		Data: req.Data,
	}

	if err := s.store.Update(table, record); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	table := r.PathValue("table")
	id := r.PathValue("id")

	if table == "" || id == "" {
		writeError(w, http.StatusBadRequest, "table and id are required")
		return
	}

	if err := s.store.Delete(table, id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	table := r.PathValue("table")
	if table == "" {
		writeError(w, http.StatusBadRequest, "table is required")
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if _, err := fmt.Sscanf(l, "%d", &limit); err != nil {
			limit = 100
		}
	}

	records, err := s.store.List(table, limit)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, records)
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	table := r.PathValue("table")
	if table == "" {
		writeError(w, http.StatusBadRequest, "table is required")
		return
	}

	field := r.URL.Query().Get("field")
	value := r.URL.Query().Get("value")
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if _, err := fmt.Sscanf(l, "%d", &limit); err != nil {
			limit = 100
		}
	}

	if field == "" || value == "" {
		writeError(w, http.StatusBadRequest, "field and value query params are required")
		return
	}

	records, err := s.store.Query(table, field, value, limit)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, records)
}

func (s *Server) handleCount(w http.ResponseWriter, r *http.Request) {
	table := r.PathValue("table")
	if table == "" {
		writeError(w, http.StatusBadRequest, "table is required")
		return
	}

	count, err := s.store.Count(table)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

func (s *Server) handleWAL(w http.ResponseWriter, r *http.Request) {
	since := r.URL.Query().Get("since")

	var entries []*WALEntry
	if since != "" {
		entries = s.store.WALSince(since)
	} else {
		entries = s.store.WAL()
	}

	if entries == nil {
		entries = []*WALEntry{}
	}

	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleTruncateWAL(w http.ResponseWriter, r *http.Request) {
	s.store.TruncateWAL()
	writeJSON(w, http.StatusOK, map[string]string{"status": "truncated"})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.Stats())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "qmd",
	})
}

func (s *Server) handleClear(w http.ResponseWriter, r *http.Request) {
	s.store.Clear()
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}
