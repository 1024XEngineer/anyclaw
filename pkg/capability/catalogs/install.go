package agentstore

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

var installHTTPClient = &http.Client{Timeout: 60 * time.Second}

type InstallSpec struct {
	Skill   *SkillInstallSpec       `json:"skill,omitempty"`
	Profile *ProfileInstallSettings `json:"profile,omitempty"`
}

type InstallSource struct {
	LocalPath  string `json:"local_path,omitempty"`
	ArchiveURL string `json:"archive_url,omitempty"`
	Subdir     string `json:"subdir,omitempty"`
}

type SkillInstallSpec struct {
	Name           string            `json:"name,omitempty"`
	Description    string            `json:"description,omitempty"`
	Version        string            `json:"version,omitempty"`
	Source         *InstallSource    `json:"source,omitempty"`
	Prompts        map[string]string `json:"prompts,omitempty"`
	Permissions    []string          `json:"permissions,omitempty"`
	Entrypoint     string            `json:"entrypoint,omitempty"`
	SourceLabel    string            `json:"source_label,omitempty"`
	Registry       string            `json:"registry,omitempty"`
	Homepage       string            `json:"homepage,omitempty"`
	InstallCommand string            `json:"install_command,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type ProfileInstallSettings struct {
	AttachSkillToAgent *bool `json:"attach_skill_to_agent,omitempty"`
}

type installReceipt struct {
	PackageID         string   `json:"package_id"`
	InstalledAt       string   `json:"installed_at"`
	SkillDirs         []string `json:"skill_dirs,omitempty"`
	SkillNames        []string `json:"skill_names,omitempty"`
	PluginDirs        []string `json:"plugin_dirs,omitempty"`
	PluginNames       []string `json:"plugin_names,omitempty"`
	EnabledPlugins    []string `json:"enabled_plugins,omitempty"`
	ProfileName       string   `json:"profile_name,omitempty"`
	ProfileSkillNames []string `json:"profile_skill_names,omitempty"`
}

type installSummary struct {
	SkillName string
	SkillDir  string
}

type stagedBundle struct {
	root    string
	cleanup func() error
}

type skillFile struct {
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	Version        string            `json:"version,omitempty"`
	Prompts        map[string]string `json:"prompts,omitempty"`
	Permissions    []string          `json:"permissions,omitempty"`
	Entrypoint     string            `json:"entrypoint,omitempty"`
	Source         string            `json:"source,omitempty"`
	Registry       string            `json:"registry,omitempty"`
	Homepage       string            `json:"homepage,omitempty"`
	InstallCommand string            `json:"install_command,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

func (sm *storeManager) installPackage(pkg AgentPackage) error {
	spec := effectiveInstallSpec(pkg)
	if spec == nil {
		return nil
	}

	cfg, err := config.Load(sm.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	resolveInstallConfigPaths(cfg, sm.configPath)
	if err := sm.ensureInstallTargets(cfg); err != nil {
		return err
	}

	receipt := &installReceipt{
		PackageID:   pkg.ID,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}

	rollback := func() {
		_, _ = sm.cleanupReceiptResources(cfg, receipt)
		_ = os.Remove(sm.receiptPath(pkg.ID))
	}

	summary, err := sm.installAssets(pkg, cfg, spec, receipt)
	if err != nil {
		rollback()
		return err
	}

	configChanged := sm.configureInstalledAssets(cfg, spec, summary, receipt)

	if err := sm.saveReceipt(receipt); err != nil {
		rollback()
		return err
	}
	if configChanged {
		if err := cfg.Save(sm.configPath); err != nil {
			rollback()
			return fmt.Errorf("save config: %w", err)
		}
	}
	return nil
}

func (sm *storeManager) installAssets(pkg AgentPackage, cfg *config.Config, spec *InstallSpec, receipt *installReceipt) (installSummary, error) {
	summary := installSummary{}
	if spec.Skill != nil {
		skillName, skillDir, err := sm.installSkill(cfg, pkg, spec.Skill)
		if err != nil {
			return summary, err
		}
		summary.SkillName = skillName
		summary.SkillDir = skillDir
		receipt.SkillNames = append(receipt.SkillNames, skillName)
		receipt.SkillDirs = append(receipt.SkillDirs, skillDir)
	}
	return summary, nil
}

func (sm *storeManager) installSkill(cfg *config.Config, pkg AgentPackage, spec *SkillInstallSpec) (string, string, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		name = strings.TrimSpace(pkg.ID)
	}
	if name == "" {
		return "", "", fmt.Errorf("skill name is required")
	}

	targetDir := filepath.Join(cfg.Skills.Dir, name)
	if exists, err := dirExists(targetDir); err != nil {
		return "", "", err
	} else if exists {
		return "", "", fmt.Errorf("skill directory already exists: %s", targetDir)
	}

	if spec.Source != nil {
		staged, err := sm.stageBundle(pkg, *spec.Source)
		if err != nil {
			return "", "", err
		}
		defer staged.cleanup()
		if err := copyDirectory(staged.root, targetDir); err != nil {
			return "", "", err
		}
		if _, err := os.Stat(filepath.Join(targetDir, "skill.json")); err != nil {
			if _, mdErr := os.Stat(filepath.Join(targetDir, "SKILL.md")); mdErr == nil {
				if err := createSkillJSONFromMarkdown(targetDir); err != nil {
					return "", "", err
				}
			} else {
				if err := writeSkillDefinition(targetDir, buildSkillDefinition(pkg, spec, name)); err != nil {
					return "", "", err
				}
			}
		}
	} else {
		if err := writeSkillDefinition(targetDir, buildSkillDefinition(pkg, spec, name)); err != nil {
			return "", "", err
		}
	}

	skillName, err := installedSkillName(targetDir)
	if err != nil {
		return "", "", err
	}
	return skillName, targetDir, nil
}

func (sm *storeManager) configureInstalledAssets(cfg *config.Config, spec *InstallSpec, summary installSummary, receipt *installReceipt) bool {
	changed := false
	if summary.SkillName != "" && shouldAttachSkill(spec) {
		profileName, updated := attachSkillToProfile(cfg, summary.SkillName)
		if updated {
			receipt.ProfileName = profileName
			receipt.ProfileSkillNames = append(receipt.ProfileSkillNames, summary.SkillName)
			changed = true
		}
	}
	return changed
}

func (sm *storeManager) uninstallPackage(pkg AgentPackage) error {
	receipt, err := sm.loadReceipt(pkg.ID)
	if err != nil {
		return err
	}
	if receipt == nil {
		return nil
	}

	cfg, err := config.Load(sm.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	resolveInstallConfigPaths(cfg, sm.configPath)

	configChanged, err := sm.cleanupReceiptResources(cfg, receipt)
	if err != nil {
		return err
	}
	if configChanged {
		if err := cfg.Save(sm.configPath); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
	}
	if err := os.Remove(sm.receiptPath(pkg.ID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (sm *storeManager) cleanupReceiptResources(cfg *config.Config, receipt *installReceipt) (bool, error) {
	configChanged := false
	for _, dir := range receipt.PluginDirs {
		if err := removeInstalledDir(cfg.Plugins.Dir, dir); err != nil {
			return false, err
		}
	}
	for _, dir := range receipt.SkillDirs {
		if err := removeInstalledDir(cfg.Skills.Dir, dir); err != nil {
			return false, err
		}
	}
	if len(receipt.EnabledPlugins) > 0 && removeEnabledPlugins(cfg, receipt.EnabledPlugins) {
		configChanged = true
	}
	if receipt.ProfileName != "" && len(receipt.ProfileSkillNames) > 0 && detachSkillsFromProfile(cfg, receipt.ProfileName, receipt.ProfileSkillNames) {
		configChanged = true
	}
	return configChanged, nil
}

func effectiveInstallSpec(pkg AgentPackage) *InstallSpec {
	spec := cloneInstallSpec(pkg.Install)
	if spec == nil {
		spec = &InstallSpec{}
	}
	if spec.Skill == nil {
		spec.Skill = generatedSkillSpec(pkg)
	}
	if spec.Skill == nil {
		return nil
	}
	return spec
}

func generatedSkillSpec(pkg AgentPackage) *SkillInstallSpec {
	systemPrompt := strings.TrimSpace(pkg.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = buildGeneratedSkillPrompt(pkg)
	}
	if systemPrompt == "" && strings.TrimSpace(pkg.Description) == "" {
		return nil
	}
	return &SkillInstallSpec{
		Name:           firstNonEmpty(pkg.Name, pkg.ID),
		Description:    strings.TrimSpace(pkg.Description),
		Version:        firstNonEmpty(pkg.Version, "1.0.0"),
		SourceLabel:    "agentstore",
		Registry:       "agentstore",
		InstallCommand: "anyclaw store install " + strings.TrimSpace(pkg.ID),
		Prompts: map[string]string{
			"system": systemPrompt,
		},
		Metadata: compactStringMap(map[string]string{
			"author":     pkg.Author,
			"category":   pkg.Category,
			"display":    pkg.DisplayName,
			"domain":     pkg.Domain,
			"permission": pkg.Permission,
		}),
	}
}

func buildGeneratedSkillPrompt(pkg AgentPackage) string {
	parts := make([]string, 0, 5)
	if value := strings.TrimSpace(pkg.DisplayName); value != "" {
		parts = append(parts, "Role: "+value)
	}
	if value := strings.TrimSpace(pkg.Persona); value != "" {
		parts = append(parts, "Persona: "+value)
	}
	if value := strings.TrimSpace(pkg.Domain); value != "" {
		parts = append(parts, "Domain: "+value)
	}
	if len(pkg.Expertise) > 0 {
		parts = append(parts, "Expertise: "+strings.Join(pkg.Expertise, ", "))
	}
	if value := strings.TrimSpace(pkg.Description); value != "" {
		parts = append(parts, "Description: "+value)
	}
	return strings.Join(parts, "\n")
}

func cloneInstallSpec(spec *InstallSpec) *InstallSpec {
	if spec == nil {
		return nil
	}
	cloned := &InstallSpec{
		Skill: cloneSkillInstallSpec(spec.Skill),
	}
	if spec.Profile != nil {
		cloned.Profile = &ProfileInstallSettings{AttachSkillToAgent: spec.Profile.AttachSkillToAgent}
	}
	return cloned
}

func cloneSkillInstallSpec(spec *SkillInstallSpec) *SkillInstallSpec {
	if spec == nil {
		return nil
	}
	return &SkillInstallSpec{
		Name:           spec.Name,
		Description:    spec.Description,
		Version:        spec.Version,
		Source:         cloneInstallSource(spec.Source),
		Prompts:        cloneStringMap(spec.Prompts),
		Permissions:    append([]string(nil), spec.Permissions...),
		Entrypoint:     spec.Entrypoint,
		SourceLabel:    spec.SourceLabel,
		Registry:       spec.Registry,
		Homepage:       spec.Homepage,
		InstallCommand: spec.InstallCommand,
		Metadata:       cloneStringMap(spec.Metadata),
	}
}

func cloneInstallSource(source *InstallSource) *InstallSource {
	if source == nil {
		return nil
	}
	return &InstallSource{
		LocalPath:  source.LocalPath,
		ArchiveURL: source.ArchiveURL,
		Subdir:     source.Subdir,
	}
}

func buildSkillDefinition(pkg AgentPackage, spec *SkillInstallSpec, fallbackName string) skillFile {
	definition := skillFile{
		Name:           firstNonEmpty(spec.Name, fallbackName),
		Description:    firstNonEmpty(spec.Description, pkg.Description),
		Version:        firstNonEmpty(spec.Version, pkg.Version, "1.0.0"),
		Prompts:        cloneStringMap(spec.Prompts),
		Permissions:    append([]string(nil), spec.Permissions...),
		Entrypoint:     strings.TrimSpace(spec.Entrypoint),
		Source:         firstNonEmpty(spec.SourceLabel, "agentstore"),
		Registry:       strings.TrimSpace(spec.Registry),
		Homepage:       strings.TrimSpace(spec.Homepage),
		InstallCommand: firstNonEmpty(spec.InstallCommand, "anyclaw store install "+strings.TrimSpace(pkg.ID)),
		Metadata:       compactStringMap(cloneStringMap(spec.Metadata)),
	}
	if len(definition.Prompts) == 0 {
		definition.Prompts = map[string]string{
			"system": firstNonEmpty(strings.TrimSpace(pkg.SystemPrompt), buildGeneratedSkillPrompt(pkg)),
		}
	}
	return definition
}

func writeSkillDefinition(targetDir string, definition skillFile) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(targetDir, "skill.json"), data, 0o644)
}

func createSkillJSONFromMarkdown(skillDir string) error {
	skillMDPath := filepath.Join(skillDir, "SKILL.md")
	content, err := os.ReadFile(skillMDPath)
	if err != nil {
		return fmt.Errorf("skill source missing skill.json and SKILL.md: %w", err)
	}
	lines := strings.Split(string(content), "\n")
	name := filepath.Base(skillDir)
	description := "Skill from store package"
	systemPrompt := strings.TrimSpace(string(content))
	inFrontmatter := false
	frontmatterDone := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			frontmatterDone = true
			continue
		}
		if inFrontmatter && !frontmatterDone {
			switch {
			case strings.HasPrefix(line, "name:"):
				name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			case strings.HasPrefix(line, "description:"):
				description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			}
		}
	}
	return writeSkillDefinition(skillDir, skillFile{
		Name:        name,
		Description: description,
		Version:     "1.0.0",
		Source:      "agentstore",
		Prompts: map[string]string{
			"system": systemPrompt,
		},
	})
}

func installedSkillName(skillDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(skillDir, "skill.json"))
	if err != nil {
		return "", err
	}
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", err
	}
	return firstNonEmpty(payload.Name, filepath.Base(skillDir)), nil
}

func (sm *storeManager) stageBundle(pkg AgentPackage, source InstallSource) (*stagedBundle, error) {
	if strings.TrimSpace(source.LocalPath) != "" {
		resolved, err := sm.resolveSourcePath(pkg, strings.TrimSpace(source.LocalPath))
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			root, err := selectBundleRoot(resolved, source.Subdir)
			if err != nil {
				return nil, err
			}
			return &stagedBundle{root: root, cleanup: func() error { return nil }}, nil
		}
		if strings.EqualFold(filepath.Ext(resolved), ".zip") {
			root, cleanup, err := extractArchiveToTemp(resolved, source.Subdir)
			if err != nil {
				return nil, err
			}
			return &stagedBundle{root: root, cleanup: cleanup}, nil
		}
		return nil, fmt.Errorf("unsupported local source for skill install: %s", resolved)
	}
	if strings.TrimSpace(source.ArchiveURL) != "" {
		root, cleanup, err := downloadAndExtractArchive(strings.TrimSpace(source.ArchiveURL), source.Subdir)
		if err != nil {
			return nil, err
		}
		return &stagedBundle{root: root, cleanup: cleanup}, nil
	}
	return nil, fmt.Errorf("skill source is required")
}

func (sm *storeManager) resolveSourcePath(pkg AgentPackage, sourcePath string) (string, error) {
	if filepath.IsAbs(sourcePath) {
		return filepath.Clean(sourcePath), nil
	}
	baseDir := "."
	if strings.TrimSpace(pkg.sourcePath) != "" {
		baseDir = filepath.Dir(pkg.sourcePath)
	}
	return filepath.Abs(filepath.Join(baseDir, filepath.FromSlash(sourcePath)))
}

func selectBundleRoot(root string, subdir string) (string, error) {
	baseRoot := filepath.Clean(root)
	if strings.TrimSpace(subdir) != "" {
		root = filepath.Join(baseRoot, filepath.FromSlash(strings.TrimSpace(subdir)))
		if !pathWithinBase(baseRoot, root) {
			return "", fmt.Errorf("bundle subdir escapes base directory")
		}
	}
	root = filepath.Clean(root)
	for i := 0; i < 2; i++ {
		if _, err := os.Stat(filepath.Join(root, "skill.json")); err == nil {
			return root, nil
		}
		if _, err := os.Stat(filepath.Join(root, "SKILL.md")); err == nil {
			return root, nil
		}
		child, ok, err := singleNestedDirectory(root)
		if err != nil {
			return "", err
		}
		if !ok {
			break
		}
		root = child
	}
	if _, err := os.Stat(filepath.Join(root, "skill.json")); err != nil {
		if _, mdErr := os.Stat(filepath.Join(root, "SKILL.md")); mdErr != nil {
			return "", fmt.Errorf("skill bundle missing skill.json or SKILL.md")
		}
	}
	return root, nil
}

func singleNestedDirectory(root string) (string, bool, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false, err
	}
	dirs := make([]string, 0, 1)
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(root, entry.Name()))
			continue
		}
		return "", false, nil
	}
	if len(dirs) != 1 {
		return "", false, nil
	}
	return dirs[0], true, nil
}

func downloadAndExtractArchive(rawURL string, subdir string) (string, func() error, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := installHTTPClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("download failed: status %d", resp.StatusCode)
	}
	archiveFile, err := os.CreateTemp("", "agentstore-*.zip")
	if err != nil {
		return "", nil, err
	}
	defer archiveFile.Close()
	if _, err := io.Copy(archiveFile, resp.Body); err != nil {
		_ = os.Remove(archiveFile.Name())
		return "", nil, err
	}
	return extractArchiveToTemp(archiveFile.Name(), subdir)
}

func extractArchiveToTemp(archivePath string, subdir string) (string, func() error, error) {
	tempDir, err := os.MkdirTemp("", "agentstore-bundle-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() error {
		_ = os.Remove(archivePath)
		return os.RemoveAll(tempDir)
	}
	if err := extractZipArchive(archivePath, tempDir); err != nil {
		_ = cleanup()
		return "", nil, err
	}
	root, err := selectBundleRoot(tempDir, subdir)
	if err != nil {
		_ = cleanup()
		return "", nil, err
	}
	return root, cleanup, nil
}

func extractZipArchive(zipPath string, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
		targetPath := filepath.Join(destDir, file.Name)
		if !pathWithinBase(destDir, targetPath) {
			return fmt.Errorf("zip entry escapes destination: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			src.Close()
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			src.Close()
			dst.Close()
			return err
		}
		src.Close()
		dst.Close()
	}
	return nil
}

func copyDirectory(srcDir string, destDir string) error {
	srcDir = filepath.Clean(srcDir)
	destDir = filepath.Clean(destDir)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported in store bundles: %s", path)
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		targetPath := filepath.Join(destDir, rel)
		if !pathWithinBase(destDir, targetPath) {
			return fmt.Errorf("copied path escapes destination: %s", rel)
		}
		if info.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		return copyFile(path, targetPath, info.Mode())
	})
}

func copyFile(srcPath string, destPath string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

func attachSkillToProfile(cfg *config.Config, skillName string) (string, bool) {
	index := installProfileIndex(cfg)
	if index < 0 {
		return "", false
	}
	profile := &cfg.Agent.Profiles[index]
	for i := range profile.Skills {
		if strings.EqualFold(strings.TrimSpace(profile.Skills[i].Name), strings.TrimSpace(skillName)) {
			if !profile.Skills[i].Enabled {
				profile.Skills[i].Enabled = true
				return profile.Name, true
			}
			return profile.Name, false
		}
	}
	profile.Skills = append(profile.Skills, config.AgentSkillRef{Name: skillName, Enabled: true})
	return profile.Name, true
}

func detachSkillsFromProfile(cfg *config.Config, profileName string, skillNames []string) bool {
	if strings.TrimSpace(profileName) == "" || len(skillNames) == 0 {
		return false
	}
	targets := map[string]struct{}{}
	for _, name := range skillNames {
		name = strings.TrimSpace(strings.ToLower(name))
		if name != "" {
			targets[name] = struct{}{}
		}
	}
	for i := range cfg.Agent.Profiles {
		if !strings.EqualFold(strings.TrimSpace(cfg.Agent.Profiles[i].Name), strings.TrimSpace(profileName)) {
			continue
		}
		filtered := make([]config.AgentSkillRef, 0, len(cfg.Agent.Profiles[i].Skills))
		changed := false
		for _, skill := range cfg.Agent.Profiles[i].Skills {
			if _, ok := targets[strings.TrimSpace(strings.ToLower(skill.Name))]; ok {
				changed = true
				continue
			}
			filtered = append(filtered, skill)
		}
		if changed {
			cfg.Agent.Profiles[i].Skills = filtered
		}
		return changed
	}
	return false
}

func removeEnabledPlugins(cfg *config.Config, pluginNames []string) bool {
	if len(cfg.Plugins.Enabled) == 0 || len(pluginNames) == 0 {
		return false
	}
	targets := map[string]struct{}{}
	for _, name := range pluginNames {
		name = strings.TrimSpace(strings.ToLower(name))
		if name != "" {
			targets[name] = struct{}{}
		}
	}
	filtered := make([]string, 0, len(cfg.Plugins.Enabled))
	changed := false
	for _, current := range cfg.Plugins.Enabled {
		if _, ok := targets[strings.TrimSpace(strings.ToLower(current))]; ok {
			changed = true
			continue
		}
		filtered = append(filtered, current)
	}
	if changed {
		cfg.Plugins.Enabled = filtered
	}
	return changed
}

func installProfileIndex(cfg *config.Config) int {
	active := strings.TrimSpace(cfg.Agent.ActiveProfile)
	if active != "" {
		for i, profile := range cfg.Agent.Profiles {
			if profile.IsEnabled() && strings.EqualFold(strings.TrimSpace(profile.Name), active) {
				return i
			}
		}
	}
	for i, profile := range cfg.Agent.Profiles {
		if profile.IsEnabled() {
			return i
		}
	}
	return -1
}

func shouldAttachSkill(spec *InstallSpec) bool {
	if spec == nil || spec.Profile == nil {
		return true
	}
	if spec.Profile.AttachSkillToAgent == nil {
		return true
	}
	return *spec.Profile.AttachSkillToAgent
}

func (sm *storeManager) ensureInstallTargets(cfg *config.Config) error {
	if strings.TrimSpace(cfg.Skills.Dir) == "" {
		return nil
	}
	if err := os.MkdirAll(cfg.Skills.Dir, 0o755); err != nil {
		return err
	}
	return nil
}

func (sm *storeManager) receiptPath(id string) string {
	return filepath.Join(sm.installDir, "store", "receipts", id+".json")
}

func (sm *storeManager) saveReceipt(receipt *installReceipt) error {
	if receipt == nil {
		return nil
	}
	path := sm.receiptPath(receipt.PackageID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (sm *storeManager) loadReceipt(id string) (*installReceipt, error) {
	data, err := os.ReadFile(sm.receiptPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var receipt installReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func removeInstalledDir(baseDir string, target string) error {
	baseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	if !pathWithinBase(baseDir, target) {
		return fmt.Errorf("refusing to remove path outside install base: %s", target)
	}
	if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func dirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

func pathWithinBase(baseDir string, targetPath string) bool {
	baseDir = filepath.Clean(baseDir)
	targetPath = filepath.Clean(targetPath)
	if baseDir == targetPath {
		return true
	}
	return strings.HasPrefix(targetPath, baseDir+string(os.PathSeparator))
}

func normalizeInstallDirName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "package"
	}
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-", ":", "-", ".", "-")
	value = replacer.Replace(value)
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	value = strings.Trim(value, "-")
	if value == "" {
		return "package"
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func resolveInstallConfigPaths(cfg *config.Config, configPath string) {
	if cfg == nil {
		return
	}
	if resolved := config.ResolvePath(configPath, cfg.Skills.Dir); resolved != "" {
		cfg.Skills.Dir = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Plugins.Dir); resolved != "" {
		cfg.Plugins.Dir = resolved
	}
}

func containsFold(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

func compactStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}
	result := make(map[string]string, len(items))
	for key, value := range items {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func cloneStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(items))
	for key, value := range items {
		cloned[key] = value
	}
	return cloned
}

func cloneAnyMap(items map[string]any) map[string]any {
	if len(items) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(items))
	for key, value := range items {
		switch typed := value.(type) {
		case map[string]any:
			cloned[key] = cloneAnyMap(typed)
		case []any:
			cloned[key] = cloneAnySlice(typed)
		default:
			cloned[key] = typed
		}
	}
	return cloned
}

func cloneAnySlice(items []any) []any {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]any, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case map[string]any:
			cloned = append(cloned, cloneAnyMap(typed))
		case []any:
			cloned = append(cloned, cloneAnySlice(typed))
		default:
			cloned = append(cloned, typed)
		}
	}
	return cloned
}
