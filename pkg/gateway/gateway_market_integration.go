package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
)

type marketIntegrationReceipt struct {
	ArtifactID   string   `json:"artifact_id"`
	ReceiptID    string   `json:"receipt_id"`
	Kind         string   `json:"kind"`
	SkillName    string   `json:"skill_name,omitempty"`
	SkillDir     string   `json:"skill_dir,omitempty"`
	AgentName    string   `json:"agent_name,omitempty"`
	CLIName      string   `json:"cli_name,omitempty"`
	CLIRoot      string   `json:"cli_root,omitempty"`
	CLIRegistry  string   `json:"cli_registry,omitempty"`
	CreatedPaths []string `json:"created_paths,omitempty"`
	CreatedAt    string   `json:"created_at"`
}

type marketArtifactManifest struct {
	ID              string            `json:"id"`
	Kind            string            `json:"kind"`
	Name            string            `json:"name"`
	Summary         string            `json:"summary,omitempty"`
	Description     string            `json:"description,omitempty"`
	DescriptionMD   string            `json:"description_md,omitempty"`
	Version         string            `json:"version"`
	Publisher       string            `json:"publisher,omitempty"`
	Permissions     []string          `json:"permissions,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	ManifestSummary map[string]string `json:"manifest_summary,omitempty"`
}

func (s *Server) integrateMarketJob(job *marketplace.InstallJob) {
	if s == nil || job == nil || job.State != marketplace.JobSucceeded || strings.TrimSpace(job.ReceiptID) == "" {
		return
	}
	receipt, err := s.marketplaceStore().GetReceipt(job.ReceiptID)
	if err != nil {
		s.appendMarketIntegrationEvent("market.integration.failed", marketplace.MarketEvent{
			Level:      "error",
			Message:    err.Error(),
			ArtifactID: job.ArtifactID,
			JobID:      job.ID,
		})
		return
	}
	record, err := s.integrateMarketReceipt(receipt)
	if err != nil {
		s.appendMarketIntegrationEvent("market.integration.failed", marketplace.MarketEvent{
			Level:      "error",
			Message:    err.Error(),
			ArtifactID: receipt.ArtifactID,
			JobID:      job.ID,
			Payload:    map[string]any{"receipt_id": receipt.ID},
		})
		return
	}
	s.appendMarketIntegrationEvent("market.integration.succeeded", marketplace.MarketEvent{
		Level:      "success",
		Message:    "Marketplace artifact integrated",
		ArtifactID: receipt.ArtifactID,
		JobID:      job.ID,
		Payload: map[string]any{
			"receipt_id": receipt.ID,
			"kind":       receipt.Kind,
			"skill_name": record.SkillName,
			"agent_name": record.AgentName,
			"cli_name":   record.CLIName,
		},
	})
}

func (s *Server) integrateMarketReceipt(receipt *marketplace.InstallReceipt) (marketIntegrationReceipt, error) {
	if s == nil || s.mainRuntime == nil || s.mainRuntime.Config == nil {
		return marketIntegrationReceipt{}, fmt.Errorf("runtime config is not available")
	}
	if receipt == nil {
		return marketIntegrationReceipt{}, fmt.Errorf("install receipt is nil")
	}
	manifest := readMarketArtifactManifest(receipt.InstalledPath)
	record := marketIntegrationReceipt{
		ArtifactID: receipt.ArtifactID,
		ReceiptID:  receipt.ID,
		Kind:       string(receipt.Kind),
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	switch receipt.Kind {
	case marketplace.ArtifactKindSkill:
		if err := s.integrateMarketSkill(receipt, manifest, &record); err != nil {
			return record, err
		}
	case marketplace.ArtifactKindAgent:
		if err := s.integrateMarketAgent(receipt, manifest, &record); err != nil {
			return record, err
		}
	case marketplace.ArtifactKindCLI:
		if err := s.integrateMarketCLI(receipt, manifest, &record); err != nil {
			return record, err
		}
	default:
		return record, fmt.Errorf("unsupported marketplace artifact kind: %s", receipt.Kind)
	}
	if err := s.saveMarketIntegration(record); err != nil {
		return record, err
	}
	return record, nil
}

func (s *Server) integrateMarketSkill(receipt *marketplace.InstallReceipt, manifest marketArtifactManifest, record *marketIntegrationReceipt) error {
	skillName := firstNonEmpty(manifest.Name, receipt.Name, receipt.ArtifactID)
	skillDirName := safeMarketIntegrationName(firstNonEmpty(receipt.ArtifactID, skillName))
	skillsDir := config.ResolvePath(s.mainRuntime.ConfigPath, s.mainRuntime.Config.Skills.Dir)
	if skillsDir == "" {
		skillsDir = config.ResolvePath(s.mainRuntime.ConfigPath, "skills")
	}
	targetDir := filepath.Join(skillsDir, skillDirName)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	sourceSkillDir := filepath.Join(receipt.InstalledPath, "skill")
	sourceHasSkillJSON := hasFile(sourceSkillDir, "skill.json")
	if sourceHasSkillJSON || hasFile(sourceSkillDir, "SKILL.md") {
		if err := copyDirContents(sourceSkillDir, targetDir); err != nil {
			return err
		}
	}
	if !hasFile(targetDir, "skill.json") {
		// The registry-generated packages currently ship SKILL.md without frontmatter.
		// Writing skill.json keeps the runtime catalog identity aligned with registry metadata.
		if err := writeMarketSkillJSON(targetDir, receipt, manifest, skillName); err != nil {
			return err
		}
	}
	record.SkillName = skillName
	record.SkillDir = targetDir
	record.CreatedPaths = append(record.CreatedPaths, targetDir)
	return s.attachSkillToMainProfile(skillName, receipt.Version, receipt.Permissions)
}

func (s *Server) integrateMarketAgent(receipt *marketplace.InstallReceipt, manifest marketArtifactManifest, record *marketIntegrationReceipt) error {
	agentName := firstNonEmpty(manifest.Name, receipt.Name, receipt.ArtifactID)
	profile := config.AgentProfile{
		Name:            agentName,
		Description:     firstNonEmpty(manifest.Summary, manifest.Description, receipt.Description),
		Role:            "marketplace",
		Persona:         firstNonEmpty(manifest.DescriptionMD, manifest.Description, manifest.Summary, receipt.Description),
		Domain:          "marketplace",
		Expertise:       append([]string(nil), manifest.Tags...),
		WorkingDir:      s.mainRuntime.Config.Agent.WorkingDir,
		PermissionLevel: firstNonEmpty(permissionLevelFrom(receipt.Permissions), s.mainRuntime.Config.Agent.PermissionLevel, "limited"),
		ProviderRef:     s.mainRuntime.Config.LLM.DefaultProviderRef,
		Enabled:         config.BoolPtr(true),
	}
	if strings.TrimSpace(profile.Persona) == "" {
		profile.Persona = "Marketplace installed agent: " + receipt.ArtifactID
	}
	profile.SystemPrompt = profile.Persona
	if err := s.mainRuntime.Config.UpsertAgentProfile(profile); err != nil {
		return err
	}
	if err := s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath); err != nil {
		return err
	}
	record.AgentName = agentName
	return nil
}

func (s *Server) integrateMarketCLI(receipt *marketplace.InstallReceipt, manifest marketArtifactManifest, record *marketIntegrationReceipt) error {
	root := filepath.Join(s.mainRuntime.WorkingDir, "CLI-Anything")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	entryName := safeMarketIntegrationName(firstNonEmpty(manifest.ManifestSummary["command"], manifest.Name, receipt.Name, receipt.ArtifactID))
	commandDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(commandDir, 0o755); err != nil {
		return err
	}
	commandPath := filepath.Join(commandDir, entryName+".cmd")
	entry := map[string]any{
		"name":         entryName,
		"display_name": firstNonEmpty(manifest.Name, receipt.Name, entryName),
		"version":      firstNonEmpty(receipt.Version, manifest.Version),
		"description":  firstNonEmpty(manifest.Summary, manifest.Description, receipt.Description),
		"entry_point":  commandPath,
		"category":     "marketplace",
		"contributor":  firstNonEmpty(manifest.Publisher, "AnyClaw Cloud"),
	}
	registryPath := filepath.Join(root, "registry.json")
	if err := upsertMarketCLIRegistryEntry(registryPath, entryName, entry); err != nil {
		return err
	}
	if err := os.WriteFile(commandPath, []byte("@echo off\r\nrem AnyClaw marketplace CLI placeholder\r\n"), 0o755); err != nil {
		return err
	}
	record.CLIName = entryName
	record.CLIRoot = root
	record.CLIRegistry = registryPath
	record.CreatedPaths = append(record.CreatedPaths, commandPath)
	return nil
}

func (s *Server) cleanupMarketIntegration(result *marketplace.UninstallResult) {
	if s == nil || result == nil {
		return
	}
	record, err := s.loadMarketIntegration(result.ReceiptID)
	if err != nil {
		return
	}
	switch record.Kind {
	case string(marketplace.ArtifactKindSkill):
		_ = removePathWithin(config.ResolvePath(s.mainRuntime.ConfigPath, s.mainRuntime.Config.Skills.Dir), record.SkillDir)
		_ = s.detachSkillFromMainProfile(record.SkillName)
	case string(marketplace.ArtifactKindAgent):
		if strings.TrimSpace(record.AgentName) != "" && s.mainRuntime.Config.DeleteAgentProfile(record.AgentName) {
			_ = s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath)
		}
	case string(marketplace.ArtifactKindCLI):
		_ = removeMarketCLIRegistryEntry(record.CLIRegistry, record.CLIName)
		for _, path := range record.CreatedPaths {
			_ = removePathWithin(record.CLIRoot, path)
		}
	}
	_ = os.Remove(s.marketIntegrationPath(result.ReceiptID))
}

func (s *Server) attachSkillToMainProfile(name, version string, permissions []string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	if profile, ok := s.mainRuntime.Config.ResolveMainAgentProfile(); ok {
		found := false
		for i := range profile.Skills {
			if strings.EqualFold(strings.TrimSpace(profile.Skills[i].Name), strings.TrimSpace(name)) {
				profile.Skills[i].Enabled = true
				if strings.TrimSpace(profile.Skills[i].Version) == "" {
					profile.Skills[i].Version = version
				}
				found = true
				break
			}
		}
		if !found {
			profile.Skills = append(profile.Skills, config.AgentSkillRef{Name: name, Enabled: true, Version: version, Permissions: append([]string(nil), permissions...)})
		}
		if err := s.mainRuntime.Config.UpsertAgentProfile(profile); err != nil {
			return err
		}
		return s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath)
	}
	for i := range s.mainRuntime.Config.Agent.Skills {
		if strings.EqualFold(strings.TrimSpace(s.mainRuntime.Config.Agent.Skills[i].Name), strings.TrimSpace(name)) {
			s.mainRuntime.Config.Agent.Skills[i].Enabled = true
			return s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath)
		}
	}
	s.mainRuntime.Config.Agent.Skills = append(s.mainRuntime.Config.Agent.Skills, config.AgentSkillRef{Name: name, Enabled: true, Version: version, Permissions: append([]string(nil), permissions...)})
	return s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath)
}

func (s *Server) detachSkillFromMainProfile(name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	if profile, ok := s.mainRuntime.Config.ResolveMainAgentProfile(); ok {
		profile.Skills = filterSkillRefs(profile.Skills, name)
		if err := s.mainRuntime.Config.UpsertAgentProfile(profile); err != nil {
			return err
		}
		return s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath)
	}
	s.mainRuntime.Config.Agent.Skills = filterSkillRefs(s.mainRuntime.Config.Agent.Skills, name)
	return s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath)
}

func filterSkillRefs(refs []config.AgentSkillRef, name string) []config.AgentSkillRef {
	filtered := make([]config.AgentSkillRef, 0, len(refs))
	for _, ref := range refs {
		if strings.EqualFold(strings.TrimSpace(ref.Name), strings.TrimSpace(name)) {
			continue
		}
		filtered = append(filtered, ref)
	}
	return filtered
}

func readMarketArtifactManifest(root string) marketArtifactManifest {
	var manifest marketArtifactManifest
	data, err := os.ReadFile(filepath.Join(root, "anyclaw.artifact.json"))
	if err != nil {
		return manifest
	}
	_ = json.Unmarshal(data, &manifest)
	return manifest
}

func writeMarketSkillJSON(targetDir string, receipt *marketplace.InstallReceipt, manifest marketArtifactManifest, skillName string) error {
	prompt := firstNonEmpty(manifest.DescriptionMD, manifest.Description, manifest.Summary, receipt.Description, "Marketplace installed skill: "+receipt.ArtifactID)
	payload := map[string]any{
		"name":            skillName,
		"description":     firstNonEmpty(manifest.Summary, manifest.Description, receipt.Description),
		"version":         firstNonEmpty(receipt.Version, manifest.Version, "1.0.0"),
		"permissions":     receipt.Permissions,
		"source":          "marketplace",
		"registry":        receipt.SourceID,
		"install_command": "anyclaw market install " + receipt.ArtifactID,
		"prompts":         map[string]string{"system": prompt},
		"metadata": map[string]string{
			"artifact_id": receipt.ArtifactID,
			"receipt_id":  receipt.ID,
			"source":      "marketplace",
		},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(targetDir, "skill.json"), data, 0o644)
}

func upsertMarketCLIRegistryEntry(path, name string, entry map[string]any) error {
	var registry struct {
		Meta map[string]string `json:"meta"`
		CLIs []map[string]any  `json:"clis"`
	}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &registry)
	}
	if registry.Meta == nil {
		registry.Meta = map[string]string{"repo": "AnyClaw Marketplace", "description": "AnyClaw marketplace CLI entries", "updated": time.Now().UTC().Format(time.RFC3339)}
	}
	replaced := false
	for i := range registry.CLIs {
		if strings.EqualFold(fmt.Sprint(registry.CLIs[i]["name"]), name) {
			registry.CLIs[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		registry.CLIs = append(registry.CLIs, entry)
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func removeMarketCLIRegistryEntry(path, name string) error {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(name) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var registry struct {
		Meta map[string]string `json:"meta"`
		CLIs []map[string]any  `json:"clis"`
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return err
	}
	filtered := make([]map[string]any, 0, len(registry.CLIs))
	for _, entry := range registry.CLIs {
		if strings.EqualFold(fmt.Sprint(entry["name"]), name) {
			continue
		}
		filtered = append(filtered, entry)
	}
	registry.CLIs = filtered
	out, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o644)
}

func (s *Server) saveMarketIntegration(record marketIntegrationReceipt) error {
	path := s.marketIntegrationPath(record.ReceiptID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (s *Server) loadMarketIntegration(receiptID string) (marketIntegrationReceipt, error) {
	var record marketIntegrationReceipt
	data, err := os.ReadFile(s.marketIntegrationPath(receiptID))
	if err != nil {
		return record, err
	}
	err = json.Unmarshal(data, &record)
	return record, err
}

func (s *Server) marketIntegrationPath(receiptID string) string {
	return filepath.Join(s.marketplaceStore().MarketplaceDir(), "integrations", safeMarketIntegrationName(receiptID)+".json")
}

func (s *Server) appendMarketIntegrationEvent(eventType string, event marketplace.MarketEvent) {
	event.Type = eventType
	_ = s.marketplaceStore().AppendEvent(event)
	_ = s.marketplaceStore().AppendAudit(marketplace.MarketAuditEvent{
		Type:       eventType,
		ArtifactID: event.ArtifactID,
		JobID:      event.JobID,
		Detail:     event.Payload,
	})
}

func hasFile(dir, name string) bool {
	info, err := os.Stat(filepath.Join(dir, name))
	return err == nil && !info.IsDir()
}

func copyDirContents(srcDir, destDir string) error {
	srcDir = filepath.Clean(srcDir)
	destDir = filepath.Clean(destDir)
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported: %s", path)
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil || rel == "." {
			return err
		}
		target := filepath.Join(destDir, rel)
		if !pathWithinBase(destDir, target) {
			return fmt.Errorf("copied path escapes destination: %s", rel)
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func removePathWithin(baseDir, target string) error {
	if strings.TrimSpace(baseDir) == "" || strings.TrimSpace(target) == "" {
		return nil
	}
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return err
	}
	resolved, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if !pathWithinBase(base, resolved) {
		return fmt.Errorf("refusing to remove path outside base: %s", resolved)
	}
	return os.RemoveAll(resolved)
}

func pathWithinBase(baseDir, targetPath string) bool {
	baseDir = filepath.Clean(baseDir)
	targetPath = filepath.Clean(targetPath)
	return targetPath == baseDir || strings.HasPrefix(targetPath, baseDir+string(os.PathSeparator))
}

func safeMarketIntegrationName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-", ".", "-")
	value = replacer.Replace(value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "market-item"
	}
	return value
}

func permissionLevelFrom(permissions []string) string {
	for _, permission := range permissions {
		p := strings.ToLower(strings.TrimSpace(permission))
		if strings.Contains(p, "process.exec") || strings.Contains(p, "fs.write") {
			return "standard"
		}
	}
	return "limited"
}
