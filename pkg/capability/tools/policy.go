package tools

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	PrivacyScopePublic     = "public"
	PrivacyScopeWork       = "work"
	PrivacyScopePersonal   = "personal"
	PrivacyScopeSystem     = "system"
	PrivacyScopeRestricted = "restricted"
)

const (
	DataScopeRead    = "read"
	DataScopeWrite   = "write"
	DataScopeDelete  = "delete"
	DataScopeExecute = "execute"
	DataScopeEgress  = "egress"
)

type PrivacyDomain int

const (
	PrivacyDomainNone PrivacyDomain = iota
	PrivacyDomainBrowser
	PrivacyDomainChat
	PrivacyDomainCredentials
	PrivacyDomainKeys
	PrivacyDomainDocuments
	PrivacyDomainMedia
	PrivacyDomainSystem
	PrivacyDomainNetwork
)

var privacyPathPatterns = map[PrivacyDomain][]string{
	PrivacyDomainBrowser: {
		`.*[\\/]Google[\\/]Chrome[\\/]Default[\\/]Login.*`,
		`.*[\\/]Google[\\/]Chrome[\\/]Default[\\/]Network[\\/]Cookies.*`,
		`.*[\\/]Microsoft[\\/]Edge[\\/]Default[\\/]Login.*`,
		`.*[\\/]Microsoft[\\/]Edge[\\/]Default[\\/]Network[\\/]Cookies.*`,
		`.*[\\/]Mozilla[\\/]Firefox[\\/]profiles[\\/].*[\\/]logins.json.*`,
		`.*[\\/]Safari[\\/].*[\\/]Cookies.plist.*`,
	},
	PrivacyDomainChat: {
		`.*[\\/]Tencent[\\/]Files[\\/].*[\\/]Msg.*`,
		`.*[\\/]WeChat[\\/].*[\\/]Msg.*`,
		`.*[\\/]QQ[\\/]Users[\\/].*[\\/]Msg.*`,
		`.*[\\/]Telegram[\\/]tdata.*`,
		`.*[\\/]Discord[\\/].*[\\/]messages.*`,
		`.*[\\/]Slack[\\/].*[\\/]Cache.*`,
	},
	PrivacyDomainCredentials: {
		`.*[\\/]\.netrc.*`,
		`.*[\\/]git-credentials.*`,
		`.*[\\/]\.aws[\\/]credentials.*`,
		`.*[\\/]keychain.*`,
		`.*[\\/]KeePass.*`,
		`.*[\\/]1Password.*`,
	},
	PrivacyDomainKeys: {
		`.*[\\/]\.ssh[\\/].*`,
		`.*[\\/]gnupg[\\/].*`,
		`.*[\\/]\.gnupg[\\/].*`,
		`.*[\\/]id_rsa.*`,
		`.*[\\/]id_ed25519.*`,
		`.*[\\/]\.pem.*`,
	},
	PrivacyDomainDocuments: {
		`.*[\\/]Documents[\\/]Personal.*`,
		`.*[\\/]Desktop[\\/]Private.*`,
		`.*[\\/]Downloads[\\/].*`,
	},
	PrivacyDomainMedia: {
		`.*[\\/]Pictures[\\/]Camera Roll.*`,
		`.*[\\/]Videos[\\/].*`,
		`.*[\\/]Photos[\\/].*`,
	},
	PrivacyDomainSystem: {
		`.*[\\/]Windows[\\/]System32[\\/].*`,
		`.*[\\/]Windows[\\/]SysWOW64[\\/].*`,
		`.*[\\/]Program Files[\\/].*`,
		`.*[\\/]ProgramData[\\/].*`,
		`.*[\\/]\.config[\\/]system.*`,
	},
}

var dangerousCommandPatterns = []string{
	`rm\s+-rf\s+`,
	`del\s+/[fqs]\s+`,
	`format\s+`,
	`dd\s+if=`,
	`mkfs\.`,
	`fdisk\s+-`,
	`shutdown`,
	`reboot`,
	`reg\s+(delete|add)`,
	`sc\s+(delete|create|config)`,
	`powershell\s+-Command\s+.*-Object\s+.*Start-Process`,
	`curl\|sh`,
	`wget\s+\|`,
	`chmod\s+777`,
	`chown\s+.*\s+root`,
	`sudo\s+rm`,
	`:\(){:|:&};:`,
}

type PolicyOptions struct {
	WorkingDir           string
	PermissionLevel      string
	ProtectedPaths       []string
	AllowedReadPaths     []string
	AllowedWritePaths    []string
	AllowedEgressDomains []string
}

type PolicyEngine struct {
	workingDir           string
	permissionLevel      string
	protectedPaths       []string
	allowedReadPaths     []string
	allowedWritePaths    []string
	allowedEgressDomains []string
}

type PrivacyEngine struct {
	workingDir        string
	allowedEgressDoms []string
	blockedEgressDoms []string
	privacyPathCache  map[string]PrivacyDomain
	dangerousCmdRegex []*regexp.Regexp
	privacyRegexCache map[PrivacyDomain][]*regexp.Regexp
}

type PrivacyCheckResult struct {
	IsAllowed        bool
	Domain           PrivacyDomain
	DomainName       string
	RequiresApproval bool
	RiskLevel        string
	Reason           string
}

type RiskLabel struct {
	Name        string
	Severity    string
	Description string
}

func NewPolicyEngine(opts PolicyOptions) *PolicyEngine {
	engine := &PolicyEngine{
		permissionLevel: strings.TrimSpace(strings.ToLower(opts.PermissionLevel)),
	}
	engine.workingDir = normalizePolicyPath(resolvePath(opts.WorkingDir, ""))
	engine.protectedPaths = normalizePolicyPaths(opts.ProtectedPaths, opts.WorkingDir)
	engine.allowedReadPaths = normalizePolicyPaths(opts.AllowedReadPaths, opts.WorkingDir)
	engine.allowedWritePaths = normalizePolicyPaths(opts.AllowedWritePaths, opts.WorkingDir)
	engine.allowedEgressDomains = normalizePolicyDomains(opts.AllowedEgressDomains)
	return engine
}

type PrivacyOptions struct {
	WorkingDir           string
	AllowedEgressDomains []string
	BlockedEgressDomains []string
}

func NewPrivacyEngine(opts PrivacyOptions) *PrivacyEngine {
	engine := &PrivacyEngine{
		workingDir:        normalizePolicyPath(resolvePath(opts.WorkingDir, "")),
		allowedEgressDoms: normalizePolicyDomains(opts.AllowedEgressDomains),
		blockedEgressDoms: normalizePolicyDomains(opts.BlockedEgressDomains),
		privacyPathCache:  make(map[string]PrivacyDomain),
		dangerousCmdRegex: make([]*regexp.Regexp, 0, len(dangerousCommandPatterns)),
		privacyRegexCache: make(map[PrivacyDomain][]*regexp.Regexp),
	}
	for _, pattern := range dangerousCommandPatterns {
		if re, err := regexp.Compile("(?i)" + pattern); err == nil {
			engine.dangerousCmdRegex = append(engine.dangerousCmdRegex, re)
		}
	}
	return engine
}

func (pe *PrivacyEngine) ClassifyPath(path string) PrivacyDomain {
	path = normalizePolicyPath(path)
	if domain, ok := pe.privacyPathCache[path]; ok {
		return domain
	}
	for domain := range privacyPathPatterns {
		if pe.matchPathPatterns(path, domain) {
			pe.privacyPathCache[path] = domain
			return domain
		}
	}
	return PrivacyDomainNone
}

func (pe *PrivacyEngine) matchPathPatterns(path string, domain PrivacyDomain) bool {
	patterns, ok := privacyPathPatterns[domain]
	if !ok {
		return false
	}
	if regexes, ok := pe.privacyRegexCache[domain]; ok {
		for _, re := range regexes {
			if re.MatchString(path) {
				return true
			}
		}
		return false
	}
	var regexes []*regexp.Regexp
	for _, pattern := range patterns {
		if re, err := regexp.Compile("(?i)" + pattern); err == nil {
			regexes = append(regexes, re)
			if re.MatchString(path) {
				pe.privacyRegexCache[domain] = regexes
				return true
			}
		}
	}
	pe.privacyRegexCache[domain] = regexes
	return false
}

func (pe *PrivacyEngine) CheckPath(path string) PrivacyCheckResult {
	domain := pe.ClassifyPath(path)
	return pe.evaluateDomain(domain, path)
}

func (pe *PrivacyEngine) evaluateDomain(domain PrivacyDomain, path string) PrivacyCheckResult {
	result := PrivacyCheckResult{
		Domain:     domain,
		DomainName: domain.String(),
	}
	switch domain {
	case PrivacyDomainNone:
		result.IsAllowed = true
		result.RiskLevel = "low"
		result.RequiresApproval = false
		result.Reason = "path is in public scope"
	case PrivacyDomainBrowser, PrivacyDomainChat, PrivacyDomainCredentials, PrivacyDomainKeys:
		result.IsAllowed = false
		result.RiskLevel = "high"
		result.RequiresApproval = true
		result.Reason = fmt.Sprintf("accessing %s requires explicit approval", domain.String())
	case PrivacyDomainDocuments, PrivacyDomainMedia:
		result.IsAllowed = false
		result.RiskLevel = "medium"
		result.RequiresApproval = true
		result.Reason = fmt.Sprintf("accessing %s may contain personal data", domain.String())
	case PrivacyDomainSystem:
		result.IsAllowed = false
		result.RiskLevel = "high"
		result.RequiresApproval = true
		result.Reason = "modifying system files is restricted"
	case PrivacyDomainNetwork:
		result.IsAllowed = true
		result.RiskLevel = "medium"
		result.RequiresApproval = true
		result.Reason = "network access requires approval"
	}
	return result
}

func (pe *PrivacyEngine) CheckCommand(command string) (bool, string) {
	for _, re := range pe.dangerousCmdRegex {
		if re.MatchString(command) {
			return false, fmt.Sprintf("dangerous command pattern detected: %s", re.String())
		}
	}
	return true, ""
}

func (pe *PrivacyEngine) CheckEgress(targetURL string) PrivacyCheckResult {
	result := PrivacyCheckResult{
		Domain:     PrivacyDomainNetwork,
		DomainName: "network",
	}
	parsed, err := url.Parse(targetURL)
	if err != nil {
		result.IsAllowed = false
		result.RiskLevel = "high"
		result.Reason = fmt.Sprintf("invalid URL: %v", err)
		return result
	}
	host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
	if host == "" {
		result.IsAllowed = false
		result.RiskLevel = "high"
		result.Reason = "empty hostname"
		return result
	}
	for _, blocked := range pe.blockedEgressDoms {
		if domainMatches(host, blocked) {
			result.IsAllowed = false
			result.RiskLevel = "high"
			result.RequiresApproval = true
			result.Reason = fmt.Sprintf("egress to %s is blocked", host)
			return result
		}
	}
	if isLocalEgressHost(host) {
		result.IsAllowed = true
		result.RiskLevel = "low"
		result.Reason = "local network access"
		return result
	}
	for _, allowed := range pe.allowedEgressDoms {
		if domainMatches(host, allowed) {
			result.IsAllowed = true
			result.RiskLevel = "low"
			result.RequiresApproval = false
			result.Reason = fmt.Sprintf("egress to %s is allowed", host)
			return result
		}
	}
	result.IsAllowed = false
	result.RiskLevel = "high"
	result.RequiresApproval = true
	result.Reason = fmt.Sprintf("egress to %s is not in allowed domains", host)
	return result
}

func (d PrivacyDomain) String() string {
	switch d {
	case PrivacyDomainNone:
		return "none"
	case PrivacyDomainBrowser:
		return "browser"
	case PrivacyDomainChat:
		return "chat"
	case PrivacyDomainCredentials:
		return "credentials"
	case PrivacyDomainKeys:
		return "keys"
	case PrivacyDomainDocuments:
		return "documents"
	case PrivacyDomainMedia:
		return "media"
	case PrivacyDomainSystem:
		return "system"
	case PrivacyDomainNetwork:
		return "network"
	default:
		return "unknown"
	}
}

func (p *PolicyEngine) CheckReadPath(path string) error {
	return p.checkPathAccess(path, p.allowedReadPaths, "read")
}

func (p *PolicyEngine) CheckWritePath(path string) error {
	if strings.TrimSpace(strings.ToLower(p.permissionLevel)) == "read-only" {
		return fmt.Errorf("permission denied: current agent is read-only")
	}
	return p.checkPathAccess(path, p.allowedWritePaths, "write")
}

func (p *PolicyEngine) CheckCommandCwd(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if strings.TrimSpace(strings.ToLower(p.permissionLevel)) == "read-only" {
		return fmt.Errorf("permission denied: current agent is read-only")
	}
	return p.checkPathAccess(path, p.allowedWritePaths, "execute")
}

func (p *PolicyEngine) CheckBrowserUpload(path string, targetURL string) error {
	if err := p.CheckReadPath(path); err != nil {
		return err
	}
	return p.CheckEgressURL(targetURL)
}

func (p *PolicyEngine) CheckEgressURL(targetURL string) error {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return fmt.Errorf("browser upload denied: target page is unknown")
	}
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("browser upload denied: invalid target url: %w", err)
	}
	host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
	if host == "" || parsed.Scheme == "file" || isLocalEgressHost(host) {
		return nil
	}
	for _, allowed := range p.allowedEgressDomains {
		if domainMatches(host, allowed) {
			return nil
		}
	}
	return fmt.Errorf("egress denied: %s is not in security.allowed_egress_domains", host)
}

func (p *PolicyEngine) ValidatePluginPermissions(pluginName string, permissions []string) error {
	if len(permissions) == 0 {
		return nil
	}
	for _, permission := range permissions {
		if strings.EqualFold(strings.TrimSpace(permission), "net:out") && len(p.allowedEgressDomains) == 0 {
			return fmt.Errorf("plugin %s requests net:out but no security.allowed_egress_domains are configured", strings.TrimSpace(pluginName))
		}
	}
	return nil
}

func (p *PolicyEngine) checkPathAccess(path string, allowed []string, action string) error {
	if p == nil {
		return nil
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	target := normalizePolicyPath(path)
	if target == "" {
		return fmt.Errorf("failed to resolve %s path: %s", action, path)
	}
	for _, candidate := range allowed {
		if candidate != "" && pathWithin(target, candidate) {
			return nil
		}
	}
	for _, protected := range p.protectedPaths {
		if protected != "" && pathWithin(target, protected) {
			return fmt.Errorf("%s denied: protected path %s", action, path)
		}
	}
	if p.workingDir != "" && pathWithin(target, p.workingDir) {
		return nil
	}
	return fmt.Errorf("%s denied outside working directory: %s", action, path)
}

func normalizePolicyPaths(paths []string, workingDir string) []string {
	if len(paths) == 0 {
		return nil
	}
	items := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, item := range paths {
		normalized := normalizePolicyPath(resolvePath(item, workingDir))
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		items = append(items, normalized)
	}
	return items
}

func normalizePolicyPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	normalized, err := normalizePathForCompare(path)
	if err != nil {
		return ""
	}
	return normalized
}

func normalizePolicyDomains(domains []string) []string {
	if len(domains) == 0 {
		return nil
	}
	items := make([]string, 0, len(domains))
	seen := map[string]bool{}
	for _, item := range domains {
		item = strings.TrimSpace(strings.ToLower(item))
		item = strings.TrimPrefix(item, "https://")
		item = strings.TrimPrefix(item, "http://")
		item = strings.TrimSuffix(item, "/")
		item = strings.TrimSpace(strings.Split(item, "/")[0])
		item = strings.TrimSpace(strings.Split(item, ":")[0])
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		items = append(items, item)
	}
	return items
}

func domainMatches(host string, allowed string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	allowed = strings.TrimSpace(strings.ToLower(allowed))
	if host == "" || allowed == "" {
		return false
	}
	if host == allowed {
		return true
	}
	if strings.HasPrefix(allowed, "*.") {
		suffix := strings.TrimPrefix(allowed, "*.")
		return host == suffix || strings.HasSuffix(host, "."+suffix)
	}
	return strings.HasSuffix(host, "."+allowed)
}

func isLocalEgressHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return true
	}
	if host == "localhost" || strings.HasSuffix(host, ".local") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	return false
}

func normalizePolicyArtifactPath(path string, workingDir string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return resolvePath(path, workingDir)
}
