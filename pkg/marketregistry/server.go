package marketregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ServerConfig struct {
	Addr                string
	DataDir             string
	DBDriver            string
	DBDSN               string
	AdminToken          string
	RequireAdminToken   bool
	Seed                bool
	MaxRequestBodyBytes int64
}

type Server struct {
	mux                 *http.ServeMux
	store               *Store
	storage             PackageStorage
	addr                string
	adminToken          string
	maxRequestBodyBytes int64
}

func NewServer(ctx context.Context, cfg ServerConfig) (*Server, error) {
	if cfg.RequireAdminToken && strings.TrimSpace(cfg.AdminToken) == "" {
		return nil, fmt.Errorf("admin token is required")
	}
	store, err := OpenStoreWithConfig(ctx, StoreConfig{DataDir: cfg.DataDir, Driver: cfg.DBDriver, DSN: cfg.DBDSN})
	if err != nil {
		return nil, err
	}
	storage, err := NewLocalStorage(cfg.DataDir)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	if cfg.Seed {
		if err := SeedIfEmpty(ctx, store, storage); err != nil {
			_ = store.Close()
			return nil, err
		}
	}
	s := &Server{
		mux:                 http.NewServeMux(),
		store:               store,
		storage:             storage,
		addr:                cfg.Addr,
		adminToken:          strings.TrimSpace(cfg.AdminToken),
		maxRequestBodyBytes: cfg.MaxRequestBodyBytes,
	}
	if s.addr == "" {
		s.addr = ":8791"
	}
	if s.maxRequestBodyBytes <= 0 {
		s.maxRequestBodyBytes = 2 << 20
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	return s.store.Close()
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) StartWithContext(ctx context.Context) error {
	server := &http.Server{
		Addr:              s.addr,
		Handler:           s,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	return server.ListenAndServe()
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /v1/health", s.handleHealth)
	s.mux.HandleFunc("GET /v1/control-plane", s.handleControlPlane)
	s.mux.HandleFunc("GET /v1/sources", s.handleSources)
	s.mux.HandleFunc("GET /v1/artifacts", s.handleListArtifacts)
	s.mux.HandleFunc("GET /v1/artifacts/{id}", s.handleArtifactDetail)
	s.mux.HandleFunc("GET /v1/artifacts/{id}/versions", s.handleArtifactVersions)
	s.mux.HandleFunc("POST /v1/artifacts/{id}/resolve", s.handleResolveArtifact)
	s.mux.HandleFunc("GET /v1/download/{artifact_id}/{version}", s.handleDownload)
	s.mux.HandleFunc("POST /v1/search", s.handleSearch)
	s.mux.HandleFunc("POST /v1/publish", s.handlePublish)
	s.mux.HandleFunc("DELETE /v1/admin/artifacts/{id}", s.handleDeleteArtifact)
	s.mux.HandleFunc("GET /v1/admin/tokens", s.handlePublisherTokens)
	s.mux.HandleFunc("POST /v1/admin/tokens", s.handleCreatePublisherToken)
	s.mux.HandleFunc("POST /v1/admin/tokens/{id}/revoke", s.handleRevokePublisherToken)
	s.mux.HandleFunc("POST /v1/artifacts/{id}/quarantine", s.handleQuarantine)
	s.mux.HandleFunc("POST /v1/artifacts/{id}/unquarantine", s.handleUnquarantine)
	s.mux.HandleFunc("GET /v1/admin/audit", s.handleAdminAudit)
	s.mux.HandleFunc("GET /v1/admin/downloads", s.handleAdminDownloads)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, map[string]string{"status": "ok", "service": "anyclaw-registry"}, 0)
}

func (s *Server) handleControlPlane(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, map[string]any{
		"protocol_version": defaultProtocolVersion,
		"artifact_kinds":   []ArtifactKind{ArtifactKindAgent, ArtifactKindSkill, ArtifactKindCLI},
		"routes": []string{
			"GET /v1/sources",
			"GET /v1/artifacts",
			"GET /v1/artifacts/{id}",
			"GET /v1/artifacts/{id}/versions",
			"POST /v1/artifacts/{id}/resolve",
			"GET /v1/download/{artifact_id}/{version}",
			"POST /v1/search",
			"POST /v1/publish",
			"DELETE /v1/admin/artifacts/{id}",
			"GET /v1/admin/tokens",
			"POST /v1/artifacts/{id}/quarantine",
			"POST /v1/artifacts/{id}/unquarantine",
			"POST /v1/admin/tokens/{id}/revoke",
			"GET /v1/admin/audit",
			"GET /v1/admin/downloads",
		},
		"admin_auth": "bearer",
	}, 0)
}

func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, map[string]any{
		"items": []map[string]any{
			{
				"id":          defaultRegistrySourceID,
				"name":        "AnyClaw Cloud",
				"trust_level": "verified",
				"kinds":       []ArtifactKind{ArtifactKindAgent, ArtifactKindSkill, ArtifactKindCLI},
			},
		},
	}, 1)
}

func (s *Server) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	filter := filterFromQuery(r)
	result, err := s.store.List(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed", "failed to list artifacts", err.Error())
		return
	}
	writeData(w, http.StatusOK, result, len(result.Items))
}

func (s *Server) handleArtifactDetail(w http.ResponseWriter, r *http.Request) {
	artifact, err := s.store.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, artifact, 1)
}

func (s *Server) handleArtifactVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := s.store.Versions(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, VersionListResult{Items: versions, Total: len(versions)}, len(versions))
}

func (s *Server) handleResolveArtifact(w http.ResponseWriter, r *http.Request) {
	var req ResolveRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	artifact, version, err := s.store.Resolve(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	resolved := ResolvedArtifact{
		ArtifactID:     artifact.ID,
		Version:        version.Version,
		DownloadURL:    absoluteURL(r, "/v1/download/"+url.PathEscape(artifact.ID)+"/"+url.PathEscape(version.Version)),
		ChecksumSHA256: version.ChecksumSHA256,
		Signature:      version.Signature,
		SizeBytes:      version.SizeBytes,
		ManifestURL:    absoluteURL(r, "/v1/artifacts/"+url.PathEscape(artifact.ID)),
		Compatibility:  version.Compatibility,
		Dependencies:   artifact.Dependencies,
		RiskLevel:      artifact.RiskLevel,
		TrustLevel:     artifact.TrustLevel,
		Permissions:    version.Permissions,
		Kind:           artifact.Kind,
		Name:           artifact.Name,
	}
	writeData(w, http.StatusOK, resolved, 1)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	artifactID := r.PathValue("artifact_id")
	versionID := r.PathValue("version")
	if _, err := s.store.Quarantine(r.Context(), artifactID); err == nil {
		writeStoreError(w, ErrArtifactUnavailable)
		return
	}
	version, err := s.store.Version(r.Context(), artifactID, versionID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if version.StorageKey == "" {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found", "")
		return
	}
	file, err := s.storage.Open(version.StorageKey)
	if err != nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found", err.Error())
		return
	}
	defer file.Close()
	_ = s.store.RecordDownload(r.Context(), artifactID, versionID, r.RemoteAddr, r.UserAgent())

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.zip"`, artifactID, versionID))
	w.Header().Set("X-Checksum-SHA256", version.ChecksumSHA256)
	if version.Signature != "" {
		w.Header().Set("X-Artifact-Signature", version.Signature)
	}
	w.Header().Set("X-Artifact-ID", artifactID)
	w.Header().Set("X-Artifact-Version", versionID)
	w.Header().Set("Content-Length", strconv.FormatInt(version.SizeBytes, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	var filter SearchFilter
	if !s.decodeJSON(w, r, &filter) {
		return
	}
	result, err := s.store.List(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search_failed", "failed to search artifacts", err.Error())
		return
	}
	writeData(w, http.StatusOK, result, len(result.Items))
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	publisherID, ok := s.authorizePublisher(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "publisher token required", "")
		return
	}
	var req PublishRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Artifact.Publisher) == "" {
		req.Artifact.Publisher = publisherID
	}
	if len(req.Versions) == 0 && strings.TrimSpace(req.Artifact.LatestVersion) != "" {
		req.Versions = []ArtifactVersion{{ArtifactID: req.Artifact.ID, Version: req.Artifact.LatestVersion}}
	}
	for i := range req.Versions {
		if req.Versions[i].ArtifactID == "" {
			req.Versions[i].ArtifactID = req.Artifact.ID
		}
		info, err := s.storage.EnsurePackage(req.Artifact, req.Versions[i])
		if err != nil {
			writeError(w, http.StatusBadRequest, "package_failed", "failed to prepare package", err.Error())
			return
		}
		req.Versions[i].StorageKey = info.StorageKey
		if req.Versions[i].SizeBytes == 0 {
			req.Versions[i].SizeBytes = info.SizeBytes
		}
		if req.Versions[i].ChecksumSHA256 == "" {
			req.Versions[i].ChecksumSHA256 = info.ChecksumSHA256
		}
	}
	if err := s.store.UpsertArtifact(r.Context(), req.Artifact, req.Versions); err != nil {
		writeError(w, http.StatusBadRequest, "publish_failed", "failed to publish artifact", err.Error())
		return
	}
	_ = s.store.AppendAudit(r.Context(), RegistryAuditEvent{Event: "artifact.published", Artifact: req.Artifact.ID, Detail: map[string]any{"publisher_id": publisherID, "versions": len(req.Versions)}})
	writeData(w, http.StatusOK, req.Artifact, 1)
}

func (s *Server) handleCreatePublisherToken(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "admin token required", "")
		return
	}
	var req struct {
		PublisherID string `json:"publisher_id"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	token, err := s.store.CreatePublisherToken(r.Context(), req.PublisherID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "token_failed", "failed to create publisher token", err.Error())
		return
	}
	writeData(w, http.StatusOK, token, 1)
}

func (s *Server) handlePublisherTokens(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "admin token required", "")
		return
	}
	result, err := s.store.PublisherTokens(r.Context(), parseInt(r.URL.Query().Get("limit"), 100))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tokens_failed", "failed to list publisher tokens", err.Error())
		return
	}
	writeData(w, http.StatusOK, result, len(result.Items))
}

func (s *Server) handleRevokePublisherToken(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "admin token required", "")
		return
	}
	record, err := s.store.RevokePublisherToken(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, record, 1)
}

func (s *Server) handleDeleteArtifact(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "admin token required", "")
		return
	}
	record, err := s.store.DeleteArtifact(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, record, 1)
}

func (s *Server) handleQuarantine(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "admin token required", "")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	record, err := s.store.SetQuarantine(r.Context(), r.PathValue("id"), req.Reason)
	if err != nil {
		writeError(w, http.StatusBadRequest, "quarantine_failed", "failed to quarantine artifact", err.Error())
		return
	}
	writeData(w, http.StatusOK, record, 1)
}

func (s *Server) handleUnquarantine(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "admin token required", "")
		return
	}
	if err := s.store.ClearQuarantine(r.Context(), r.PathValue("id")); err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]string{"status": "unquarantined"}, 1)
}

func (s *Server) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "admin token required", "")
		return
	}
	result, err := s.store.AuditEvents(r.Context(), parseInt(r.URL.Query().Get("limit"), 100))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "audit_failed", "failed to list audit events", err.Error())
		return
	}
	writeData(w, http.StatusOK, result, len(result.Items))
}

func (s *Server) handleAdminDownloads(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "admin token required", "")
		return
	}
	result, err := s.store.DownloadStats(r.Context(), parseInt(r.URL.Query().Get("limit"), 100))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "downloads_failed", "failed to list download stats", err.Error())
		return
	}
	writeData(w, http.StatusOK, result, len(result.Items))
}

func (s *Server) decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body", err.Error())
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body", "")
		return false
	}
	return true
}

func filterFromQuery(r *http.Request) SearchFilter {
	q := r.URL.Query()
	return SearchFilter{
		Kind:       ArtifactKind(strings.TrimSpace(q.Get("kind"))),
		Source:     strings.TrimSpace(q.Get("source")),
		Query:      strings.TrimSpace(q.Get("q")),
		Risk:       strings.TrimSpace(q.Get("risk")),
		Trust:      strings.TrimSpace(q.Get("trust")),
		Tag:        strings.TrimSpace(q.Get("tag")),
		Permission: strings.TrimSpace(q.Get("permission")),
		Publisher:  strings.TrimSpace(q.Get("publisher")),
		OS:         strings.TrimSpace(q.Get("os")),
		Arch:       strings.TrimSpace(q.Get("arch")),
		Sort:       strings.TrimSpace(q.Get("sort")),
		Limit:      parseInt(q.Get("limit"), 50),
		Offset:     parseInt(q.Get("offset"), 0),
	}
}

func parseInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func (s *Server) authorizeAdmin(r *http.Request) bool {
	if s == nil || s.adminToken == "" {
		return true
	}
	return bearerToken(r) == s.adminToken
}

func (s *Server) authorizePublisher(r *http.Request) (string, bool) {
	token := bearerToken(r)
	if token == "" {
		return "", false
	}
	publisherID, ok, err := s.store.ValidatePublisherToken(r.Context(), token)
	if err != nil || !ok {
		return "", false
	}
	return publisherID, true
}

func bearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(auth[len(prefix):])
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrArtifactNotFound), errors.Is(err, ErrVersionNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error(), "")
	case errors.Is(err, ErrNoCompatibleVersion):
		writeError(w, http.StatusConflict, "no_compatible_version", err.Error(), "")
	case errors.Is(err, ErrArtifactUnavailable):
		writeError(w, http.StatusGone, "artifact_unavailable", err.Error(), "")
	default:
		writeError(w, http.StatusInternalServerError, "registry_error", "registry error", err.Error())
	}
}

func writeData(w http.ResponseWriter, status int, data any, count int) {
	writeJSON(w, status, map[string]any{
		"data": data,
		"meta": ResponseMeta{
			ProtocolVersion: defaultProtocolVersion,
			Count:           count,
		},
	})
}

func writeError(w http.ResponseWriter, status int, code, message, detail string) {
	var payload ErrorResponse
	payload.Error.Code = code
	payload.Error.Message = message
	payload.Error.Detail = detail
	writeJSON(w, status, payload)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func absoluteURL(r *http.Request, path string) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	host := r.Host
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = forwardedHost
	}
	return scheme + "://" + host + path
}
