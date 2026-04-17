package skills

import (
	"fmt"
	"sort"
	"strings"
)

type builtinSkillSeed struct {
	Name        string
	Description string
	Category    string
	Permissions []string
	Verbs       []string
}

type builtinSkillBundle struct {
	Map         map[string]string
	Definitions map[string]skillFileDefinition
}

var builtinSkillSeeds = []builtinSkillSeed{
	{Name: "coder", Description: "Code generation, debugging, review, and refactoring support", Category: "engineering", Permissions: []string{"files:read", "files:write", "tools:exec"}, Verbs: []string{"write", "debug", "review"}},
	{Name: "writer", Description: "Content writing, editing, polishing, and rewriting support", Category: "content", Permissions: []string{"prompt:extended"}, Verbs: []string{"draft", "edit", "polish"}},
	{Name: "researcher", Description: "Research, fact-checking, and source-backed information gathering", Category: "research", Permissions: []string{"web:search", "web:fetch"}, Verbs: []string{"search", "compare", "verify"}},
	{Name: "analyst", Description: "Data analysis, insights, and concise reporting", Category: "analysis", Permissions: []string{"files:read", "tools:exec"}, Verbs: []string{"analyze", "compare", "report"}},
	{Name: "translator", Description: "Translation, localization, and bilingual rewriting", Category: "language", Permissions: []string{"prompt:extended"}, Verbs: []string{"translate", "localize", "check"}},
	{Name: "devops", Description: "Deployment, CI/CD, containers, and operations guidance", Category: "engineering", Permissions: []string{"tools:exec", "sandbox:run"}, Verbs: []string{"deploy", "monitor", "automate"}},
	{Name: "architect", Description: "System design, tradeoff analysis, and technical architecture planning", Category: "engineering", Permissions: []string{"prompt:extended"}, Verbs: []string{"design", "review", "scale"}},
	{Name: "find-skills", Description: "Recommend the right AnyClaw skills for a task or workflow", Category: "meta", Permissions: []string{"skills:search", "skills:install"}, Verbs: []string{"recommend", "search", "install"}},
	{Name: "product-manager", Description: "Product strategy, prioritization, and roadmap thinking", Category: "product", Permissions: []string{"prompt:extended"}, Verbs: []string{"prioritize", "scope", "roadmap"}},
	{Name: "project-planner", Description: "Execution plans, milestones, sequencing, and delivery tracking", Category: "operations", Permissions: []string{"prompt:extended"}, Verbs: []string{"plan", "sequence", "track"}},
	{Name: "qa-engineer", Description: "Quality review, regression hunting, and test gap discovery", Category: "quality", Permissions: []string{"files:read", "tools:exec"}, Verbs: []string{"review", "regress", "verify"}},
	{Name: "test-automation", Description: "Automated test design, harness planning, and fixture creation", Category: "quality", Permissions: []string{"files:read", "files:write", "tools:exec"}, Verbs: []string{"test", "automate", "cover"}},
	{Name: "security-auditor", Description: "Security review, risk assessment, and hardening advice", Category: "security", Permissions: []string{"files:read", "tools:exec"}, Verbs: []string{"audit", "harden", "assess"}},
	{Name: "data-engineer", Description: "Pipelines, transformations, ingestion, and data workflow design", Category: "data", Permissions: []string{"files:read", "files:write", "tools:exec"}, Verbs: []string{"pipeline", "transform", "ingest"}},
	{Name: "ml-engineer", Description: "Model workflow planning, evaluation, and applied ML implementation support", Category: "ai", Permissions: []string{"files:read", "files:write", "tools:exec"}, Verbs: []string{"train", "evaluate", "ship"}},
	{Name: "frontend-designer", Description: "Frontend implementation, UX polish, and component-level design", Category: "design", Permissions: []string{"files:read", "files:write"}, Verbs: []string{"layout", "style", "refine"}},
	{Name: "ui-ux-reviewer", Description: "Usability review, accessibility feedback, and interaction critique", Category: "design", Permissions: []string{"prompt:extended"}, Verbs: []string{"audit", "improve", "accessibility"}},
	{Name: "backend-engineer", Description: "Services, APIs, reliability, and server-side implementation guidance", Category: "engineering", Permissions: []string{"files:read", "files:write", "tools:exec"}, Verbs: []string{"implement", "scale", "stabilize"}},
	{Name: "api-designer", Description: "API contracts, schema design, and integration ergonomics", Category: "engineering", Permissions: []string{"files:read", "files:write"}, Verbs: []string{"design", "document", "version"}},
	{Name: "database-admin", Description: "Schema review, migrations, tuning, and database operations support", Category: "data", Permissions: []string{"files:read", "tools:exec"}, Verbs: []string{"migrate", "tune", "backup"}},
	{Name: "sre", Description: "Service reliability, incident thinking, and production guardrails", Category: "operations", Permissions: []string{"tools:exec", "sandbox:run"}, Verbs: []string{"stabilize", "observe", "recover"}},
	{Name: "release-manager", Description: "Release plans, rollout safety, and change communication", Category: "operations", Permissions: []string{"prompt:extended"}, Verbs: []string{"release", "rollout", "announce"}},
	{Name: "prompt-engineer", Description: "Prompt design, evaluation, and instruction tuning support", Category: "ai", Permissions: []string{"prompt:extended"}, Verbs: []string{"prompt", "refine", "evaluate"}},
	{Name: "workflow-builder", Description: "Reusable workflows, orchestration patterns, and automation flows", Category: "automation", Permissions: []string{"files:read", "files:write"}, Verbs: []string{"compose", "orchestrate", "reuse"}},
	{Name: "automation-engineer", Description: "Task automation across tools, scripts, apps, and systems", Category: "automation", Permissions: []string{"files:read", "files:write", "tools:exec"}, Verbs: []string{"automate", "script", "integrate"}},
	{Name: "docs-writer", Description: "Technical documentation, onboarding guides, and how-to content", Category: "content", Permissions: []string{"files:read", "files:write"}, Verbs: []string{"document", "explain", "guide"}},
	{Name: "changelog-writer", Description: "Release notes, user-facing change summaries, and upgrade guidance", Category: "content", Permissions: []string{"files:read", "files:write"}, Verbs: []string{"summarize", "announce", "format"}},
	{Name: "meeting-notes", Description: "Meeting capture, action item extraction, and follow-up summaries", Category: "operations", Permissions: []string{"prompt:extended"}, Verbs: []string{"capture", "summarize", "followup"}},
	{Name: "recruiter", Description: "Hiring plans, interview loops, and talent evaluation support", Category: "business", Permissions: []string{"prompt:extended"}, Verbs: []string{"source", "screen", "score"}},
	{Name: "sales-assistant", Description: "Sales messaging, objection handling, and pipeline support", Category: "business", Permissions: []string{"prompt:extended"}, Verbs: []string{"pitch", "qualify", "followup"}},
	{Name: "marketing-strategist", Description: "Campaign planning, positioning, and messaging strategy", Category: "business", Permissions: []string{"prompt:extended"}, Verbs: []string{"position", "campaign", "analyze"}},
	{Name: "seo-optimizer", Description: "Search optimization, content structure, and ranking-oriented review", Category: "business", Permissions: []string{"files:read", "files:write"}, Verbs: []string{"optimize", "keywords", "audit"}},
	{Name: "customer-support", Description: "Support responses, triage, and empathy-first customer handling", Category: "business", Permissions: []string{"prompt:extended"}, Verbs: []string{"triage", "respond", "resolve"}},
	{Name: "finance-analyst", Description: "Financial modeling, trend reading, and business metric interpretation", Category: "business", Permissions: []string{"files:read", "tools:exec"}, Verbs: []string{"model", "forecast", "review"}},
	{Name: "legal-assistant", Description: "Contract review support, issue spotting, and clause comparison", Category: "compliance", Permissions: []string{"files:read"}, Verbs: []string{"review", "compare", "summarize"}},
	{Name: "compliance-reviewer", Description: "Policy mapping, controls review, and compliance readiness support", Category: "compliance", Permissions: []string{"files:read"}, Verbs: []string{"map", "review", "check"}},
	{Name: "education-coach", Description: "Learning plans, study structure, and explanation-first coaching", Category: "education", Permissions: []string{"prompt:extended"}, Verbs: []string{"teach", "plan", "practice"}},
	{Name: "language-tutor", Description: "Language practice, correction, and conversational learning support", Category: "education", Permissions: []string{"prompt:extended"}, Verbs: []string{"practice", "correct", "explain"}},
	{Name: "research-paper", Description: "Paper reading, literature notes, and academic synthesis support", Category: "research", Permissions: []string{"files:read", "web:fetch"}, Verbs: []string{"read", "cite", "synthesize"}},
	{Name: "browser-automation", Description: "Web workflow planning, browser task breakdown, and verification support", Category: "automation", Permissions: []string{"tools:browser", "web:fetch"}, Verbs: []string{"browse", "extract", "verify"}},
	{Name: "app-builder", Description: "App planning, feature slicing, and implementation scaffolding support", Category: "engineering", Permissions: []string{"files:read", "files:write"}, Verbs: []string{"scaffold", "slice", "ship"}},
	{Name: "plugin-builder", Description: "Plugin design, manifest planning, and integration scaffolding", Category: "engineering", Permissions: []string{"files:read", "files:write"}, Verbs: []string{"scaffold", "manifest", "wire"}},
	{Name: "canvas-designer", Description: "Canvas layout, A2UI structure, and visual presentation support", Category: "design", Permissions: []string{"files:read", "files:write"}, Verbs: []string{"compose", "layout", "present"}},
	{Name: "voice-designer", Description: "Voice UX, speech workflow tuning, and spoken interaction design", Category: "voice", Permissions: []string{"prompt:extended"}, Verbs: []string{"voice", "tune", "script"}},
	{Name: "extension-curator", Description: "Extension discovery, packaging, and channel capability planning", Category: "meta", Permissions: []string{"files:read", "files:write"}, Verbs: []string{"catalog", "bundle", "recommend"}},
}

var builtinSkillsBundle = buildBuiltinSkillBundle()

var BuiltinSkills = builtinSkillsBundle.Map

func buildBuiltinSkillBundle() builtinSkillBundle {
	items := make(map[string]string, len(builtinSkillSeeds))
	definitions := make(map[string]skillFileDefinition, len(builtinSkillSeeds))
	for _, seed := range builtinSkillSeeds {
		definition := builtinSkillDefinition(seed)
		data, err := marshalSkillJSON(definition)
		if err != nil {
			panic(fmt.Sprintf("build builtin skill %s: %v", seed.Name, err))
		}
		items[seed.Name] = string(data)
		definitions[seed.Name] = definition
	}
	return builtinSkillBundle{
		Map:         items,
		Definitions: definitions,
	}
}

func builtinSkillDefinition(seed builtinSkillSeed) skillFileDefinition {
	name := strings.TrimSpace(seed.Name)
	description := strings.TrimSpace(seed.Description)
	return skillFileDefinition{
		Name:           name,
		Description:    description,
		Version:        "2.1.0",
		Source:         "builtin",
		Registry:       "builtin",
		Homepage:       "https://github.com/anyclaw/anyclaw",
		Entrypoint:     "builtin://" + name,
		Permissions:    append([]string(nil), seed.Permissions...),
		InstallCommand: "anyclaw skill install " + name,
		Prompts: map[string]string{
			"system": builtinSystemPrompt(seed),
		},
	}
}

func builtinSystemPrompt(seed builtinSkillSeed) string {
	verbs := make([]string, 0, len(seed.Verbs))
	for _, verb := range seed.Verbs {
		verb = strings.TrimSpace(verb)
		if verb != "" {
			verbs = append(verbs, verb)
		}
	}
	areas := "plan, execute, and review"
	if len(verbs) > 0 {
		areas = strings.Join(verbs, ", ")
	}
	return fmt.Sprintf(
		"You are the %s skill. Focus on %s. Help the user %s with crisp reasoning, clear tradeoffs, and practical next steps. Respond in Chinese unless the user asks otherwise.",
		seed.Name,
		seed.Description,
		areas,
	)
}

func BuiltinSkillDefinitions() map[string]skillFileDefinition {
	out := make(map[string]skillFileDefinition, len(builtinSkillsBundle.Definitions))
	for name, definition := range builtinSkillsBundle.Definitions {
		out[name] = definition
	}
	return out
}

func ListBuiltinSkillNames() []string {
	names := make([]string, 0, len(BuiltinSkills))
	for name := range BuiltinSkills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func GetBuiltinSkill(name string) (string, bool) {
	content, ok := BuiltinSkills[name]
	return content, ok
}

func GetBuiltinSkillDefinition(name string) (skillFileDefinition, bool) {
	definition, ok := builtinSkillsBundle.Definitions[name]
	return definition, ok
}
