package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
)

func TestRound14LiveCloudInstallClosure(t *testing.T) {
	if os.Getenv("ANYCLAW_ROUND14_LIVE") != "1" {
		t.Skip("set ANYCLAW_ROUND14_LIVE=1 to run the live Round 14 cloud marketplace closure smoke")
	}
	endpoint := strings.TrimSpace(os.Getenv("ANYCLAW_ROUND14_ENDPOINT"))
	if endpoint == "" {
		endpoint = "http://47.76.186.80/v1"
	}

	server := newMarketInstallTestServer(t, endpoint)
	server.mainRuntime.Config.Marketplace.RequestTimeoutSeconds = 10
	server.startWorkers(t.Context())

	for kind, wantID := range map[string]string{
		"agent": "anyclaw.agent.marketplace-operator",
		"skill": "anyclaw.skill.skill-author",
		"cli":   "anyclaw.cli.agent-native-runner",
	} {
		rec := httptest.NewRecorder()
		server.handleMarketArtifacts(rec, httptest.NewRequest(http.MethodGet, "/market/artifacts?source=cloud&kind="+kind+"&limit=100", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("cloud %s list status = %d body=%s", kind, rec.Code, rec.Body.String())
		}
		var payload struct {
			Data marketplace.ListResult `json:"data"`
			Meta map[string]any         `json:"meta,omitempty"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if cloudErr, _ := payload.Meta["cloud_error"].(string); cloudErr != "" {
			t.Fatalf("cloud %s list degraded: %s", kind, cloudErr)
		}
		if !marketListContains(payload.Data.Items, wantID) {
			t.Fatalf("cloud %s list missing %s: %#v", kind, wantID, payload.Data.Items)
		}
	}

	artifactID := "anyclaw.skill.skill-author"
	rec := httptest.NewRecorder()
	server.handleMarketArtifactDetail(rec, httptest.NewRequest(http.MethodGet, "/market/artifacts/"+artifactID+"?source=cloud", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("cloud detail status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	server.handleMarketArtifactDetail(rec, httptest.NewRequest(http.MethodGet, "/market/artifacts/"+artifactID+"/versions?source=cloud", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("cloud versions status = %d body=%s", rec.Code, rec.Body.String())
	}
	var versions struct {
		Data struct {
			Items []marketplace.ArtifactVersion `json:"items"`
			Total int                           `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &versions); err != nil {
		t.Fatal(err)
	}
	if versions.Data.Total != 1 || len(versions.Data.Items) != 1 || versions.Data.Items[0].Version != "1.0.0" {
		t.Fatalf("unexpected live versions: %#v", versions.Data)
	}

	installJob := startRound14Job(t, server, "/market/install", `{"artifact_id":"`+artifactID+`","user_confirmed":true}`, "round14-install")
	if installJob.State != marketplace.JobSucceeded || installJob.ReceiptID == "" {
		t.Fatalf("install job = %#v", installJob)
	}
	receipt, err := server.marketplaceStore().GetReceipt(installJob.ReceiptID)
	if err != nil {
		t.Fatalf("install receipt not found: %v", err)
	}
	if receipt.ArtifactID != artifactID || receipt.Source != marketplace.SourceCloud || receipt.ChecksumSHA256 == "" {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}

	rec = httptest.NewRecorder()
	server.handleMarketBindings(rec, httptest.NewRequest(http.MethodPost, "/market/bindings", strings.NewReader(`{"artifact_id":"`+artifactID+`","target_type":"main_agent"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("bind status = %d body=%s", rec.Code, rec.Body.String())
	}
	var bindingPayload struct {
		Data marketplace.Binding `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &bindingPayload); err != nil {
		t.Fatal(err)
	}
	if bindingPayload.Data.ArtifactID != artifactID || bindingPayload.Data.TargetType != marketplace.TargetMainAgent || bindingPayload.Data.TargetID == "" {
		t.Fatalf("unexpected binding: %#v", bindingPayload.Data)
	}

	upgradeJob := startRound14Job(t, server, "/market/upgrade", `{"artifact_id":"`+artifactID+`","user_confirmed":true}`, "round14-upgrade")
	if upgradeJob.State != marketplace.JobSucceeded || upgradeJob.Type != "upgrade" {
		t.Fatalf("upgrade job = %#v", upgradeJob)
	}
	bindings, err := server.marketplaceStore().ListBindings()
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings.Items) != 1 || bindings.Items[0].ReceiptID != upgradeJob.ReceiptID {
		t.Fatalf("binding was not retained on upgrade: %#v", bindings.Items)
	}

	rec = httptest.NewRecorder()
	server.handleMarketArtifacts(rec, httptest.NewRequest(http.MethodGet, "/market/artifacts?source=cloud&kind=skill&limit=100", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("post-upgrade cloud list status = %d body=%s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Data marketplace.ListResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if status := marketStatusFor(listed.Data.Items, artifactID); status != marketplace.StatusActive {
		t.Fatalf("post-bind cloud overlay status = %q, want active", status)
	}

	rec = httptest.NewRecorder()
	server.handleMarketUninstall(rec, httptest.NewRequest(http.MethodPost, "/market/uninstall", strings.NewReader(`{"artifact_id":"`+artifactID+`"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("uninstall status = %d body=%s", rec.Code, rec.Body.String())
	}
	var uninstall struct {
		Data marketplace.UninstallResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &uninstall); err != nil {
		t.Fatal(err)
	}
	if uninstall.Data.ArtifactID != artifactID || len(uninstall.Data.RemovedBindings) != 1 {
		t.Fatalf("unexpected uninstall: %#v", uninstall.Data)
	}
	if _, err := server.marketplaceStore().LatestReceiptForArtifact(artifactID); err != marketplace.ErrArtifactNotFound {
		t.Fatalf("receipt should be removed after uninstall, got err=%v", err)
	}
	bindings, err = server.marketplaceStore().ListBindings()
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings.Items) != 0 {
		t.Fatalf("bindings should be empty after uninstall: %#v", bindings.Items)
	}

	jobs, err := server.marketplaceStore().ListJobs(10)
	if err != nil {
		t.Fatal(err)
	}
	if jobs.Total < 2 {
		t.Fatalf("expected install and upgrade jobs, got %#v", jobs)
	}
	events, err := server.marketplaceStore().ListEvents(20)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"market.install.succeeded", "market.binding.created", "market.upgrade.started", "market.uninstall.succeeded"} {
		if !round14EventContains(events.Items, want) {
			t.Fatalf("events missing %s: %#v", want, events.Items)
		}
	}
	auditData, err := os.ReadFile(server.marketplaceStore().AuditPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"market.install.succeeded", "market.binding.created", "market.upgrade.started", "market.uninstall.succeeded"} {
		if !strings.Contains(string(auditData), want) {
			t.Fatalf("audit missing %s: %s", want, string(auditData))
		}
	}
}

func startRound14Job(t *testing.T, server *Server, path string, body string, key string) *marketplace.InstallJob {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Idempotency-Key", key+"-"+time.Now().UTC().Format("150405.000000000"))
	switch path {
	case "/market/install":
		server.handleMarketInstall(rec, req)
	case "/market/upgrade":
		server.handleMarketUpgrade(rec, req)
	default:
		t.Fatalf("unsupported job path %s", path)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
	}
	var payload struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return waitMarketJob(t, server, payload.JobID)
}

func marketListContains(items []marketplace.Artifact, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func marketStatusFor(items []marketplace.Artifact, id string) marketplace.ArtifactStatus {
	for _, item := range items {
		if item.ID == id {
			return item.Status
		}
	}
	return ""
}

func round14EventContains(items []marketplace.MarketEvent, eventType string) bool {
	for _, item := range items {
		if item.Type == eventType {
			return true
		}
	}
	return false
}
