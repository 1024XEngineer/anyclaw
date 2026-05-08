package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
	marketregistry "github.com/1024XEngineer/anyclaw/pkg/marketplace/registry"
	"github.com/1024XEngineer/anyclaw/pkg/runtime"
)

func (s *Server) handleMarketInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s == nil || s.mainRuntime == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runtime not available"})
		return
	}
	var req marketplace.InstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	req.IdempotencyKey = r.Header.Get("Idempotency-Key")
	if strings.TrimSpace(req.InstalledBy) == "" {
		req.InstalledBy = "user"
	}
	uc := marketplace.NewInstallUseCaseWithPolicy(s.marketplaceStore(), registryInstallAdapter{client: s.cloudRegistryClient()}, marketplace.PolicyConfig{
		AutoInstallSkill: s.mainRuntime.Config.Marketplace.AutoInstallSkill,
	})
	job, reused, err := uc.Start(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !reused && isTerminalMarketJob(job.State) {
		writeJSON(w, http.StatusOK, map[string]any{"job_id": job.ID, "job": job, "reused": reused})
		return
	}
	if !reused {
		run := func() {
			if err := uc.Execute(context.Background(), job.ID); err == nil {
				if done, getErr := s.marketplaceStore().GetJob(job.ID); getErr == nil {
					s.integrateMarketJob(done)
				}
			}
		}
		if s.jobQueue != nil {
			s.jobQueue <- run
		} else {
			go run()
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID, "job": job, "reused": reused})
}

func (s *Server) handleMarketUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s == nil || s.mainRuntime == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runtime not available"})
		return
	}
	var req marketplace.UpgradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	req.IdempotencyKey = r.Header.Get("Idempotency-Key")
	if strings.TrimSpace(req.InstalledBy) == "" {
		req.InstalledBy = "user"
	}
	store := s.marketplaceStore()
	job, reused, err := store.CreateUpgradeJob(req, req.IdempotencyKey)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	uc := marketplace.NewInstallUseCaseWithPolicy(store, registryInstallAdapter{client: s.cloudRegistryClient()}, marketplace.PolicyConfig{
		AutoInstallSkill: s.mainRuntime.Config.Marketplace.AutoInstallSkill,
	})
	if !reused {
		_ = store.AppendAudit(marketplace.MarketAuditEvent{
			Type:       "market.upgrade.started",
			ArtifactID: job.ArtifactID,
			JobID:      job.ID,
			Actor:      firstNonEmpty(req.InstalledBy, "user"),
			Detail: map[string]any{
				"version_constraint": req.VersionConstraint,
				"previous_version":   job.Metadata["previous_version"],
			},
		})
		_ = store.AppendEvent(marketplace.MarketEvent{
			Type:       "market.upgrade.started",
			Level:      "info",
			Message:    "Marketplace upgrade started",
			ArtifactID: job.ArtifactID,
			JobID:      job.ID,
			Payload: map[string]any{
				"version_constraint": req.VersionConstraint,
				"previous_version":   job.Metadata["previous_version"],
			},
		})
		run := func() {
			if err := uc.Execute(context.Background(), job.ID); err == nil {
				if done, getErr := s.marketplaceStore().GetJob(job.ID); getErr == nil {
					s.integrateMarketJob(done)
				}
				s.refreshMarketArtifactBindings(job.ArtifactID)
			}
		}
		if s.jobQueue != nil {
			s.jobQueue <- run
		} else {
			go run()
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID, "job": job, "reused": reused})
}

func (s *Server) handleMarketUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s == nil || s.mainRuntime == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runtime not available"})
		return
	}
	var req marketplace.UninstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if strings.TrimSpace(req.Actor) == "" {
		req.Actor = marketActor(r)
	}
	result, err := marketplace.NewLifecycleService(s.marketplaceStore()).Uninstall(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.cleanupMarketIntegration(result)
	agent, orgID, projectID, workspaceID := s.marketRefreshTarget("", "", "", "")
	_ = s.marketHotReload().Refresh(r.Context(), runtime.RefreshScope{
		Kind:      runtime.RefreshScopeRuntime,
		Agent:     agent,
		Org:       orgID,
		Project:   projectID,
		Workspace: workspaceID,
		Reason:    "market.uninstall",
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (s *Server) handleMarketJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := s.marketplaceStore().ListJobs(parseIntParam(r.URL.Query().Get("limit"), 100))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (s *Server) handleMarketJobDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/market/jobs/"), "/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "job id required"})
		return
	}
	job, err := s.marketplaceStore().GetJob(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": job})
}

func (s *Server) marketplaceStore() *marketplace.Store {
	if s.marketJobs == nil {
		root := "."
		if s != nil && s.mainRuntime != nil && s.mainRuntime.WorkDir != "" {
			root = s.mainRuntime.WorkDir
		}
		s.marketJobs = marketplace.NewStore(root)
	}
	return s.marketJobs
}

func isTerminalMarketJob(state marketplace.JobState) bool {
	return state == marketplace.JobSucceeded || state == marketplace.JobFailed || state == marketplace.JobRolledBack || state == marketplace.JobCanceled
}

type registryInstallAdapter struct {
	client *marketregistry.Client
}

func (a registryInstallAdapter) Resolve(ctx context.Context, artifactID, versionConstraint string) (marketplace.ResolvedPackage, error) {
	if a.client == nil {
		return marketplace.ResolvedPackage{}, marketregistry.ErrNotConfigured
	}
	var req marketregistry.ResolveRequest
	req.VersionConstraint = versionConstraint
	resolved, err := a.client.Resolve(ctx, artifactID, req)
	if err != nil {
		return marketplace.ResolvedPackage{}, err
	}
	return marketplace.ResolvedPackage{
		ArtifactID:     resolved.ArtifactID,
		Version:        resolved.Version,
		DownloadURL:    resolved.DownloadURL,
		ChecksumSHA256: resolved.ChecksumSHA256,
		SizeBytes:      resolved.SizeBytes,
		Compatibility:  resolved.Compatibility,
		Dependencies:   resolved.Dependencies,
		RiskLevel:      resolved.RiskLevel,
		TrustLevel:     resolved.TrustLevel,
		Permissions:    append([]string(nil), resolved.Permissions...),
		Signature:      resolved.Signature,
		Kind:           resolved.Kind,
		Name:           resolved.Name,
	}, nil
}

func (a registryInstallAdapter) Download(ctx context.Context, rawURL string) ([]byte, error) {
	if a.client == nil {
		return nil, marketregistry.ErrNotConfigured
	}
	return a.client.Download(ctx, rawURL)
}
