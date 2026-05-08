package marketplace

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type LifecycleService struct {
	store *Store
}

func NewLifecycleService(store *Store) *LifecycleService {
	return &LifecycleService{store: store}
}

func (s *LifecycleService) Uninstall(req UninstallRequest) (*UninstallResult, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("marketplace lifecycle store is not configured")
	}
	artifactID := strings.TrimSpace(req.ArtifactID)
	var receipt *InstallReceipt
	var err error
	if strings.TrimSpace(req.ReceiptID) != "" {
		receipt, err = s.store.GetReceipt(strings.TrimSpace(req.ReceiptID))
	} else {
		receipt, err = s.store.LatestReceiptForArtifact(artifactID)
	}
	if err != nil {
		return nil, err
	}
	if artifactID == "" {
		artifactID = receipt.ArtifactID
	}
	if !strings.EqualFold(strings.TrimSpace(receipt.ArtifactID), artifactID) {
		return nil, fmt.Errorf("receipt does not belong to artifact")
	}

	removedBindings, err := s.store.DeleteBindingsForArtifact(receipt.ArtifactID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(receipt.InstalledPath) != "" {
		if err := os.RemoveAll(receipt.InstalledPath); err != nil {
			return nil, err
		}
	}
	if err := s.store.DeleteReceipt(receipt.ID); err != nil {
		return nil, err
	}

	bindingIDs := make([]string, 0, len(removedBindings))
	for _, binding := range removedBindings {
		bindingIDs = append(bindingIDs, binding.ID)
	}
	result := &UninstallResult{
		ArtifactID:       receipt.ArtifactID,
		ReceiptID:        receipt.ID,
		RemovedBindings:  bindingIDs,
		RemovedPath:      receipt.InstalledPath,
		UninstalledAt:    time.Now().UTC().Format(time.RFC3339),
		PreviousVersion:  receipt.Version,
		UndoAvailableSec: 30,
	}
	s.audit("market.uninstall.succeeded", req.Actor, result, map[string]any{
		"removed_bindings": bindingIDs,
		"removed_path":     receipt.InstalledPath,
		"version":          receipt.Version,
	})
	s.event("market.uninstall.succeeded", "success", "Marketplace artifact uninstalled", result, nil)
	return result, nil
}

func (s *LifecycleService) audit(eventType, actor string, result *UninstallResult, detail map[string]any) {
	if s == nil || s.store == nil || result == nil {
		return
	}
	_ = s.store.AppendAudit(MarketAuditEvent{
		Type:       eventType,
		ArtifactID: result.ArtifactID,
		Actor:      firstNonEmpty(actor, "user"),
		Detail:     detail,
	})
}

func (s *LifecycleService) event(eventType, level, message string, result *UninstallResult, payload map[string]any) {
	if s == nil || s.store == nil || result == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["receipt_id"] = result.ReceiptID
	payload["removed_bindings"] = result.RemovedBindings
	_ = s.store.AppendEvent(MarketEvent{
		Type:       eventType,
		Level:      level,
		Message:    message,
		ArtifactID: result.ArtifactID,
		Payload:    payload,
	})
}
