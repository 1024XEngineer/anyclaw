package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/apps"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/plugin"
)

// PlannerClient 接口用于 planner 重排序
type PlannerClient interface {
	Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error)
	Name() string
}

// Router 是低token路由的核心组件
type Router struct {
	registry  *plugin.Registry
	config    *config.Config
	llmClient llm.Client
	planner   PlannerClient
	nowFunc   func() time.Time
	cache     *RouteCache
}

// TaskIntent 表示任务意图的统一数据结构
type TaskIntent struct {
	ID           string
	Title        string
	Input        string
	RawInput     string
	UserID       string
	Workspace    string
	Org          string
	Project      string
	SessionID    string
	CreatedAt    time.Time
	RiskLabels   []string
	PrivacyScope string
	DataScope    string
}

// RouteResult 路由结果
type RouteResult struct {
	Mode             string // workflow|app-action|tool-chain|planner
	Plugin           string
	Workflow         string
	App              string
	Confidence       float64
	RequiresApproval bool
	RiskLevel        string // low|medium|high
	TokenCost        int    // 预计token成本
	Explanation      string
	WorkflowMatch    *plugin.AppWorkflowMatch
}

// RouteCache 路由缓存
type RouteCache struct {
	ruleMatches   map[string]*RouteResult
	workflowCache map[string][]*plugin.AppWorkflowMatch
	templateCache map[string]*WorkflowTemplate
}

// WorkflowTemplate 工作流模板
type WorkflowTemplate struct {
	ID               string
	Name             string
	Description      string
	DefaultPlugin    string
	DefaultWorkflow  string
	Steps            []TemplateStep
	Tags             []string
	Platforms        []string
	Confidence       float64
	RequiresApproval bool
	RiskLevel        string
	EstimatedTokens  int
}

type TemplateStep struct {
	Action string
	App    string
	Inputs map[string]any
	Verify string
}

// NewRouter 创建新的路由器
func NewRouter(registry *plugin.Registry, config *config.Config, llmClient llm.Client, planner PlannerClient) *Router {
	return &Router{
		registry:  registry,
		config:    config,
		llmClient: llmClient,
		planner:   planner,
		nowFunc: func() time.Time {
			return time.Now().UTC()
		},
		cache: &RouteCache{
			ruleMatches:   make(map[string]*RouteResult),
			workflowCache: make(map[string][]*plugin.AppWorkflowMatch),
			templateCache: make(map[string]*WorkflowTemplate),
		},
	}
}

// RouteTask 路由任务
func (r *Router) RouteTask(ctx context.Context, intent TaskIntent) (*RouteResult, error) {
	// 第1步：检查缓存
	if cached, ok := r.cache.ruleMatches[intent.Input]; ok {
		return cached, nil
	}

	// 第2步：低token路由优先级
	// 1. 规则匹配（L0：零模型）
	ruleMatch := r.matchRules(ctx, intent)
	if ruleMatch != nil {
		r.cache.ruleMatches[intent.Input] = ruleMatch
		return ruleMatch, nil
	}

	// 2. App Workflow 检索
	workflowMatch := r.matchWorkflows(ctx, intent)
	if workflowMatch != nil && workflowMatch.Confidence >= 0.8 {
		return workflowMatch, nil
	}

	// 3. 小模型分类（L1：轻量分类）
	smallModelResult := r.routeWithSmallModel(ctx, intent)
	if smallModelResult != nil && smallModelResult.Confidence >= 0.6 {
		return smallModelResult, nil
	}

	// 4. 模板匹配
	templateMatch := r.matchTemplates(ctx, intent)
	if templateMatch != nil && templateMatch.Confidence >= 0.7 {
		return templateMatch, nil
	}

	// 5. 大模型规划（最后选择）
	return r.routeWithLargeModel(ctx, intent)
}

// matchRules 规则匹配（L0：零模型）
func (r *Router) matchRules(ctx context.Context, intent TaskIntent) *RouteResult {
	rules := r.getRules()
	for _, rule := range rules {
		if rule.Matches(intent.Input) {
			return &RouteResult{
				Mode:             "workflow",
				Plugin:           rule.Plugin,
				Workflow:         rule.Workflow,
				Confidence:       1.0,
				RequiresApproval: rule.RequiresApproval,
				RiskLevel:        rule.RiskLevel,
				TokenCost:        0,
				Explanation:      fmt.Sprintf("Matched rule: %s", rule.Name),
			}
		}
	}
	return nil
}

// ResolveWorkflowMatches resolves workflow matches from registry
func (r *Router) ResolveWorkflowMatches(ctx context.Context, input string, limit int) []plugin.AppWorkflowMatch {
	if r.registry == nil || strings.TrimSpace(input) == "" {
		return nil
	}
	candidates := r.registry.ResolveWorkflowMatches(input, limit)
	if len(candidates) == 0 {
		return nil
	}
	if r.planner == nil || len(candidates) <= 1 {
		return trimWorkflowMatches(candidates, 3)
	}
	selected, ok := r.RerankWorkflowMatches(ctx, input, candidates)
	if ok && len(selected) > 0 {
		return selected
	}
	return trimWorkflowMatches(candidates, 3)
}

// ResolveWorkflowMatchesWithPairings resolves workflow matches with pairings support
func (r *Router) ResolveWorkflowMatchesWithPairings(ctx context.Context, input string, limit int, pairings []any) []plugin.AppWorkflowMatch {
	if r.registry == nil || strings.TrimSpace(input) == "" {
		return nil
	}
	candidates := r.registry.ResolveWorkflowMatchesWithPairings(input, limit, r.convertPairings(pairings))
	if len(candidates) == 0 {
		return nil
	}
	if r.planner == nil || len(candidates) <= 1 {
		return trimWorkflowMatches(candidates, 3)
	}
	selected, ok := r.RerankWorkflowMatches(ctx, input, candidates)
	if ok && len(selected) > 0 {
		return selected
	}
	return trimWorkflowMatches(candidates, 3)
}

func (r *Router) convertPairings(pairings []any) []*apps.Pairing {
	if len(pairings) == 0 {
		return nil
	}
	result := make([]*apps.Pairing, 0, len(pairings))
	for _, p := range pairings {
		if pairing, ok := p.(*apps.Pairing); ok {
			result = append(result, pairing)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// RerankWorkflowMatches uses planner to rerank workflow candidates
func (r *Router) RerankWorkflowMatches(ctx context.Context, input string, candidates []plugin.AppWorkflowMatch) ([]plugin.AppWorkflowMatch, bool) {
	if r.planner == nil || len(candidates) == 0 {
		return nil, false
	}
	lines := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		line := fmt.Sprintf("- %s | app=%s | workflow=%s | action=%s | tags=%s | desc=%s",
			candidate.Workflow.ToolName,
			candidate.Workflow.App,
			candidate.Workflow.Name,
			candidate.Workflow.Action,
			strings.Join(candidate.Workflow.Tags, ", "),
			candidate.Workflow.Description,
		)
		if candidate.Pairing != nil {
			line += fmt.Sprintf(" | pairing=%s", candidate.Pairing.Name)
			if strings.TrimSpace(candidate.Pairing.Binding) != "" {
				line += fmt.Sprintf(" | binding=%s", strings.TrimSpace(candidate.Pairing.Binding))
			}
		}
		lines = append(lines, line)
	}
	messages := []llm.Message{
		{Role: "system", Content: "You rank candidate app workflows for a local assistant. Return JSON only with {\"matches\":[{\"tool_name\":\"...\",\"reason\":\"...\"}]}. Choose at most 3 tool names. Only choose from the listed tool names."},
		{Role: "user", Content: fmt.Sprintf("User request:\n%s\n\nCandidate workflows:\n%s", strings.TrimSpace(input), strings.Join(lines, "\n"))},
	}
	resp, err := r.planner.Chat(ctx, messages, nil)
	if err != nil || resp == nil || strings.TrimSpace(resp.Content) == "" {
		return nil, false
	}
	var payload struct {
		Matches []struct {
			ToolName string `json:"tool_name"`
			Reason   string `json:"reason"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &payload); err != nil {
		return nil, false
	}
	byTool := make(map[string]plugin.AppWorkflowMatch, len(candidates))
	for _, candidate := range candidates {
		byTool[candidate.Workflow.ToolName] = candidate
	}
	selected := make([]plugin.AppWorkflowMatch, 0, len(payload.Matches))
	for _, item := range payload.Matches {
		match, ok := byTool[strings.TrimSpace(item.ToolName)]
		if !ok {
			continue
		}
		if strings.TrimSpace(item.Reason) != "" {
			match.Reason = strings.TrimSpace(item.Reason)
		}
		selected = append(selected, match)
		if len(selected) >= 3 {
			break
		}
	}
	if len(selected) == 0 {
		return nil, false
	}
	return selected, true
}

func trimWorkflowMatches(matches []plugin.AppWorkflowMatch, limit int) []plugin.AppWorkflowMatch {
	if len(matches) == 0 {
		return nil
	}
	if limit <= 0 || len(matches) <= limit {
		return append([]plugin.AppWorkflowMatch(nil), matches...)
	}
	return append([]plugin.AppWorkflowMatch(nil), matches[:limit]...)
}

func extractJSON(input string) string {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "```") {
		parts := strings.Split(input, "```")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "json") {
				part = strings.TrimSpace(strings.TrimPrefix(part, "json"))
			}
			if strings.HasPrefix(part, "{") {
				return part
			}
		}
	}
	return input
}

// matchWorkflows App Workflow检索
func (r *Router) matchWorkflows(ctx context.Context, intent TaskIntent) *RouteResult {
	// 检查缓存
	if cached, ok := r.cache.workflowCache[intent.Input]; ok && len(cached) > 0 {
		bestMatch := cached[0]
		return &RouteResult{
			Mode:             "workflow",
			Plugin:           bestMatch.Workflow.Plugin,
			Workflow:         bestMatch.Workflow.Name,
			App:              bestMatch.Workflow.App,
			Confidence:       float64(bestMatch.Score) / 100.0,
			RequiresApproval: r.requiresApprovalForWorkflow(bestMatch),
			RiskLevel:        r.assessWorkflowRisk(bestMatch),
			TokenCost:        50, // 基础token成本
			WorkflowMatch:    bestMatch,
			Explanation:      fmt.Sprintf("Matched workflow: %s", bestMatch.Workflow.Name),
		}
	}

	// 使用 registry 解析 workflow matches
	matches := r.ResolveWorkflowMatches(ctx, intent.Input, 6)
	if len(matches) == 0 {
		return nil
	}

	// 缓存结果 - 转换为指针切片
	cached := make([]*plugin.AppWorkflowMatch, len(matches))
	for i := range matches {
		cached[i] = &matches[i]
	}
	r.cache.workflowCache[intent.Input] = cached

	bestMatch := &matches[0]
	return &RouteResult{
		Mode:             "workflow",
		Plugin:           bestMatch.Workflow.Plugin,
		Workflow:         bestMatch.Workflow.Name,
		App:              bestMatch.Workflow.App,
		Confidence:       float64(bestMatch.Score) / 100.0,
		RequiresApproval: r.requiresApprovalForWorkflow(bestMatch),
		RiskLevel:        r.assessWorkflowRisk(bestMatch),
		TokenCost:        50,
		WorkflowMatch:    bestMatch,
		Explanation:      fmt.Sprintf("Matched workflow: %s", bestMatch.Workflow.Name),
	}
}

type TaskCategory int

const (
	CategoryUnknown TaskCategory = iota
	CategoryCommunication
	CategoryFileOperation
	CategoryWebBrowser
	CategoryDocument
	CategorySystem
	CategoryDesktop
	CategoryDataProcessing
	CategoryNetwork
)

func (c TaskCategory) String() string {
	switch c {
	case CategoryCommunication:
		return "communication"
	case CategoryFileOperation:
		return "file_operation"
	case CategoryWebBrowser:
		return "web_browser"
	case CategoryDocument:
		return "document"
	case CategorySystem:
		return "system"
	case CategoryDesktop:
		return "desktop"
	case CategoryDataProcessing:
		return "data_processing"
	case CategoryNetwork:
		return "network"
	default:
		return "unknown"
	}
}

var categoryKeywords = map[TaskCategory][]string{
	CategoryCommunication:  {"消息", "短信", "邮件", "聊天", "发送", "发给", "qq", "微信", "telegram", "message", "send"},
	CategoryFileOperation:  {"复制", "移动", "删除", "新建", "创建", "文件夹", "文件", "copy", "move", "delete", "new", "folder", "file"},
	CategoryWebBrowser:     {"搜索", "浏览", "打开网址", "访问", "点击链接", "search", "browse", "url", "website"},
	CategoryDocument:       {"编辑", "写", "文档", "文本", "打开文件", "保存", "edit", "write", "document", "text", "save"},
	CategorySystem:         {"安装", "卸载", "设置", "配置", "系统", "install", "uninstall", "config", "setting", "system"},
	CategoryDesktop:        {"截图", "点击", "输入", "最小化", "最大化", "关闭窗口", "screenshot", "click", "type", "minimize", "maximize", "close"},
	CategoryDataProcessing: {"分析", "处理", "转换", "统计", "analyze", "process", "convert", "calculate"},
	CategoryNetwork:        {"下载", "上传", "ping", "curl", "http", "download", "upload"},
}

var categoryToPlugin = map[TaskCategory][]string{
	CategoryCommunication:  {"qq-local", "web-browser"},
	CategoryFileOperation:  {"file-manager"},
	CategoryWebBrowser:     {"web-browser"},
	CategoryDocument:       {"notepad", "office-local"},
	CategorySystem:         {"terminal", "desktop-tools"},
	CategoryDesktop:        {"desktop-tools"},
	CategoryDataProcessing: {"python", "terminal"},
	CategoryNetwork:        {"web-browser", "terminal"},
}

func init() {
	categoryKeywords = map[TaskCategory][]string{
		CategoryCommunication:  {"消息", "短信", "邮件", "聊天", "发送", "发给", "qq", "微信", "telegram", "message", "send"},
		CategoryFileOperation:  {"复制", "移动", "删除", "新建", "创建", "文件夹", "文件", "copy", "move", "delete", "new", "folder", "file"},
		CategoryWebBrowser:     {"搜索", "浏览", "打开网址", "访问", "点击链接", "search", "browse", "url", "website"},
		CategoryDocument:       {"编辑", "写", "文档", "文本", "打开文件", "保存", "edit", "write", "document", "text", "save"},
		CategorySystem:         {"安装", "卸载", "设置", "配置", "系统", "install", "uninstall", "config", "setting", "system"},
		CategoryDesktop:        {"截图", "点击", "输入", "最小化", "最大化", "关闭窗口", "screenshot", "click", "type", "minimize", "maximize", "close"},
		CategoryDataProcessing: {"分析", "处理", "转换", "统计", "analyze", "process", "convert", "calculate"},
		CategoryNetwork:        {"下载", "上传", "ping", "curl", "http", "download", "upload"},
	}
}

// routeWithSmallModel 小模型路由（L1）
// 只做轻量分类，不做长推理：判断任务类别、选择插件、补默认参数、在多个workflow中rerank
func (r *Router) routeWithSmallModel(ctx context.Context, intent TaskIntent) *RouteResult {
	category := r.classifyTask(intent.Input)
	if category == CategoryUnknown {
		return nil
	}
	plugins := categoryToPlugin[category]
	if len(plugins) == 0 {
		return nil
	}
	plugin := plugins[0]
	workflow := r.getDefaultWorkflowForCategory(category)
	return &RouteResult{
		Mode:             "app-action",
		Plugin:           plugin,
		Workflow:         workflow,
		Confidence:       0.65,
		RequiresApproval: r.requiresApprovalByCategory(category),
		RiskLevel:        r.assessCategoryRisk(category),
		TokenCost:        100,
		Explanation:      fmt.Sprintf("L1 small model classified as: %s", category.String()),
	}
}

func (r *Router) classifyTask(input string) TaskCategory {
	input = strings.ToLower(input)
	bestCategory := CategoryUnknown
	bestScore := 0
	for category, keywords := range categoryKeywords {
		score := 0
		for _, keyword := range keywords {
			if strings.Contains(input, strings.ToLower(keyword)) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestCategory = category
		}
	}
	if bestScore >= 1 {
		return bestCategory
	}
	return CategoryUnknown
}

func (r *Router) getDefaultWorkflowForCategory(category TaskCategory) string {
	switch category {
	case CategoryCommunication:
		return "send-message"
	case CategoryFileOperation:
		return "list-files"
	case CategoryWebBrowser:
		return "open_url"
	case CategoryDocument:
		return "open_file"
	case CategorySystem:
		return "run_command"
	case CategoryDesktop:
		return "click"
	case CategoryDataProcessing:
		return "run_script"
	case CategoryNetwork:
		return "download"
	default:
		return ""
	}
}

func (r *Router) requiresApprovalByCategory(category TaskCategory) bool {
	switch category {
	case CategorySystem:
		return true
	case CategoryFileOperation:
		return true
	default:
		return false
	}
}

func (r *Router) assessCategoryRisk(category TaskCategory) string {
	switch category {
	case CategorySystem:
		return "high"
	case CategoryFileOperation:
		return "medium"
	case CategoryNetwork:
		return "medium"
	default:
		return "low"
	}
}

// matchTemplates 模板匹配
func (r *Router) matchTemplates(ctx context.Context, intent TaskIntent) *RouteResult {
	// 检查缓存
	for _, template := range r.getTemplates() {
		if template.Matches(intent.Input) {
			return &RouteResult{
				Mode:             "workflow",
				Plugin:           template.DefaultPlugin,
				Workflow:         template.DefaultWorkflow,
				Confidence:       template.Confidence,
				RequiresApproval: template.RequiresApproval,
				RiskLevel:        template.RiskLevel,
				TokenCost:        template.EstimatedTokens,
				Explanation:      fmt.Sprintf("Matched template: %s", template.Name),
			}
		}
	}
	return nil
}

// routeWithLargeModel 大模型规划（L2）
func (r *Router) routeWithLargeModel(ctx context.Context, intent TaskIntent) (*RouteResult, error) {
	// 只在没有匹配的情况下使用
	return &RouteResult{
		Mode:             "planner",
		Confidence:       0.5,
		RequiresApproval: true,
		RiskLevel:        "medium",
		TokenCost:        1000, // 估计token成本
		Explanation:      "No cheap path matched, using large model planner",
	}, nil
}

// getRules 获取规则列表
func (r *Router) getRules() []RoutingRule {
	return []RoutingRule{
		// QQ消息
		{Name: "qq-message", Pattern: "发.*消息", Plugin: "qq-local", Workflow: "send-message", RequiresApproval: false, RiskLevel: "low"},
		{Name: "qq-message-cn", Pattern: "给.*发.*消息", Plugin: "qq-local", Workflow: "send-message", RequiresApproval: false, RiskLevel: "low"},

		// 文件管理
		{Name: "open-folder", Pattern: "打开.*文件夹", Plugin: "file-manager", Workflow: "open_folder", RequiresApproval: false, RiskLevel: "low"},
		{Name: "new-folder", Pattern: "新建.*文件夹|创建.*文件夹", Plugin: "file-manager", Workflow: "new_folder", RequiresApproval: false, RiskLevel: "low"},
		{Name: "copy-file", Pattern: "复制.*文件|拷贝.*文件", Plugin: "file-manager", Workflow: "copy", RequiresApproval: false, RiskLevel: "low"},
		{Name: "move-file", Pattern: "移动.*文件|剪切.*文件", Plugin: "file-manager", Workflow: "move", RequiresApproval: false, RiskLevel: "low"},
		{Name: "delete-file", Pattern: "删除.*文件", Plugin: "file-manager", Workflow: "delete", RequiresApproval: true, RiskLevel: "medium"},

		// 浏览器
		{Name: "open-url", Pattern: "打开.*网址|访问.*网站", Plugin: "web-browser", Workflow: "open_url", RequiresApproval: false, RiskLevel: "low"},
		{Name: "web-search", Pattern: "搜索.*|百度.*", Plugin: "web-browser", Workflow: "search", RequiresApproval: false, RiskLevel: "low"},
		{Name: "browser-screenshot", Pattern: "浏览器.*截图", Plugin: "web-browser", Workflow: "screenshot", RequiresApproval: false, RiskLevel: "low"},

		// 记事本
		{Name: "open-notepad", Pattern: "打开.*记事本|打开.*文本", Plugin: "notepad", Workflow: "open_file", RequiresApproval: false, RiskLevel: "low"},
		{Name: "write-notepad", Pattern: "写.*文字|输入.*内容", Plugin: "notepad", Workflow: "type_text", RequiresApproval: false, RiskLevel: "low"},
		{Name: "save-file", Pattern: "保存.*文件", Plugin: "notepad", Workflow: "save", RequiresApproval: false, RiskLevel: "low"},

		// 终端
		{Name: "run-command", Pattern: "执行.*命令|运行.*命令", Plugin: "terminal", Workflow: "execute_command", RequiresApproval: true, RiskLevel: "high"},
		{Name: "ping", Pattern: "ping.*", Plugin: "terminal", Workflow: "execute_command", RequiresApproval: false, RiskLevel: "low"},

		// 桌面操作
		{Name: "screenshot", Pattern: "截图", Plugin: "desktop-tools", Workflow: "screenshot", RequiresApproval: false, RiskLevel: "low"},
		{Name: "click", Pattern: "点击.*按钮", Plugin: "desktop-tools", Workflow: "click", RequiresApproval: false, RiskLevel: "low"},
		{Name: "type-text", Pattern: "输入.*文字|打字", Plugin: "desktop-tools", Workflow: "type", RequiresApproval: false, RiskLevel: "low"},
		{Name: "hotkey", Pattern: "按.*快捷键", Plugin: "desktop-tools", Workflow: "hotkey", RequiresApproval: false, RiskLevel: "low"},

		// 系统操作
		{Name: "open-app", Pattern: "打开.*应用", Plugin: "desktop-tools", Workflow: "open", RequiresApproval: false, RiskLevel: "low"},
		{Name: "minimize-window", Pattern: "最小化.*窗口", Plugin: "desktop-tools", Workflow: "window_minimize", RequiresApproval: false, RiskLevel: "low"},
		{Name: "maximize-window", Pattern: "最大化.*窗口", Plugin: "desktop-tools", Workflow: "window_maximize", RequiresApproval: false, RiskLevel: "low"},
		{Name: "close-window", Pattern: "关闭.*窗口", Plugin: "desktop-tools", Workflow: "window_close", RequiresApproval: false, RiskLevel: "low"},
	}
}

type RoutingRule struct {
	Name             string
	Pattern          string
	Plugin           string
	Workflow         string
	RequiresApproval bool
	RiskLevel        string
}

func (rule RoutingRule) Matches(input string) bool {
	input = strings.TrimSpace(input)
	pattern := strings.TrimSpace(rule.Pattern)
	if input == "" || pattern == "" {
		return false
	}
	if re, err := regexp.Compile("(?i)" + pattern); err == nil {
		return re.MatchString(input)
	}
	return strings.Contains(strings.ToLower(input), strings.ToLower(pattern))
}

// getTemplates 获取模板列表
func (r *Router) getTemplates() []WorkflowTemplate {
	return []WorkflowTemplate{
		{
			ID:               "file-copy-template",
			Name:             "文件复制",
			Description:      "从一个位置复制文件到另一个位置",
			DefaultPlugin:    "file-manager",
			DefaultWorkflow:  "copy-files",
			Tags:             []string{"文件", "复制", "移动"},
			Platforms:        []string{"windows", "linux"},
			Confidence:       0.8,
			RequiresApproval: false,
			RiskLevel:        "low",
			EstimatedTokens:  100,
		},
		{
			ID:               "web-search-template",
			Name:             "网页搜索",
			Description:      "在浏览器中搜索信息并保存结果",
			DefaultPlugin:    "browser-local",
			DefaultWorkflow:  "search-and-save",
			Tags:             []string{"网页", "搜索", "浏览器"},
			Platforms:        []string{"windows", "linux"},
			Confidence:       0.7,
			RequiresApproval: false,
			RiskLevel:        "low",
			EstimatedTokens:  150,
		},
		{
			ID:               "document-edit-template",
			Name:             "文档编辑",
			Description:      "打开文档编辑器并编辑内容",
			DefaultPlugin:    "office-local",
			DefaultWorkflow:  "edit-document",
			Tags:             []string{"文档", "编辑", "文字"},
			Platforms:        []string{"windows"},
			Confidence:       0.6,
			RequiresApproval: false,
			RiskLevel:        "low",
			EstimatedTokens:  200,
		},
	}
}

func (template WorkflowTemplate) Matches(input string) bool {
	for _, tag := range template.Tags {
		if strings.Contains(input, tag) {
			return true
		}
	}
	return false
}

// requiresApprovalForWorkflow 判断workflow是否需要审批
func (r *Router) requiresApprovalForWorkflow(match *plugin.AppWorkflowMatch) bool {
	// TODO: 根据workflow的风险等级和配置判断
	if match.Pairing != nil {
		return false // 已绑定的pairing通常不需要审批
	}
	return true // 默认需要审批
}

// assessWorkflowRisk 评估workflow风险等级
func (r *Router) assessWorkflowRisk(match *plugin.AppWorkflowMatch) string {
	// TODO: 根据workflow类型和权限评估风险
	workflowName := strings.ToLower(match.Workflow.Name)
	if strings.Contains(workflowName, "send") || strings.Contains(workflowName, "upload") {
		return "medium"
	}
	return "low"
}

// BuildCapabilityGraph 构建能力图谱
func (r *Router) BuildCapabilityGraph() *CapabilityGraph {
	graph := &CapabilityGraph{
		Plugins:   make(map[string]*PluginCapability),
		Workflows: make(map[string]*WorkflowCapability),
		Tags:      make(map[string][]string),
		Platforms: make(map[string][]string),
	}

	if r.registry == nil {
		return graph
	}

	manifests := r.registry.List()
	for _, manifest := range manifests {
		if !manifest.Enabled {
			continue
		}
		pluginCap := &PluginCapability{
			Name:        manifest.Name,
			Description: manifest.Description,
			Kinds:       manifest.Kinds,
			Platforms:   getPlatformsFromManifest(manifest),
			RiskLevel:   assessPluginRisk(manifest),
		}

		if manifest.App != nil {
			pluginCap.Platforms = manifest.App.Platforms
			for _, workflow := range manifest.App.Workflows {
				wfCap := WorkflowCapability{
					Name:        workflow.Name,
					Description: workflow.Description,
					Plugin:      manifest.Name,
					App:         manifest.App.Name,
					Tags:        workflow.Tags,
					InputSchema: workflow.InputSchema,
				}
				pluginCap.Workflows = append(pluginCap.Workflows, wfCap)

				key := manifest.Name + "." + workflow.Name
				graph.Workflows[key] = &wfCap

				for _, tag := range workflow.Tags {
					tag = strings.ToLower(tag)
					if _, ok := graph.Tags[tag]; !ok {
						graph.Tags[tag] = []string{}
					}
					graph.Tags[tag] = append(graph.Tags[tag], key)
				}
			}
		}

		graph.Plugins[manifest.Name] = pluginCap

		for _, platform := range pluginCap.Platforms {
			platform = strings.ToLower(platform)
			if _, ok := graph.Platforms[platform]; !ok {
				graph.Platforms[platform] = []string{}
			}
			graph.Platforms[platform] = append(graph.Platforms[platform], manifest.Name)
		}
	}

	return graph
}

func getPlatformsFromManifest(manifest plugin.Manifest) []string {
	var platforms []string
	if manifest.App != nil && len(manifest.App.Platforms) > 0 {
		platforms = manifest.App.Platforms
	}
	if manifest.Node != nil && len(manifest.Node.Platforms) > 0 {
		platforms = append(platforms, manifest.Node.Platforms...)
	}
	if len(platforms) == 0 {
		platforms = []string{"windows", "linux", "darwin"}
	}
	return uniqueStrings(platforms)
}

func assessPluginRisk(manifest plugin.Manifest) string {
	if manifest.Trust == "verified" {
		return "low"
	}
	for _, kind := range manifest.Kinds {
		if kind == "channel" || kind == "ingress" {
			return "medium"
		}
	}
	return "low"
}

func uniqueStrings(items []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

type CapabilityGraph struct {
	Plugins   map[string]*PluginCapability
	Workflows map[string]*WorkflowCapability
	Tags      map[string][]string
	Platforms map[string][]string
}

type PluginCapability struct {
	Name        string
	Description string
	Kinds       []string
	Platforms   []string
	RiskLevel   string
	Workflows   []WorkflowCapability
}

type WorkflowCapability struct {
	Name        string
	Description string
	Plugin      string
	App         string
	Tags        []string
	InputSchema map[string]any
	Confidence  float64
}

// ClearCache 清除缓存
func (r *Router) ClearCache() {
	r.cache = &RouteCache{
		ruleMatches:   make(map[string]*RouteResult),
		workflowCache: make(map[string][]*plugin.AppWorkflowMatch),
		templateCache: make(map[string]*WorkflowTemplate),
	}
}
