package agentstore

type PackageBundle struct {
	Mode          string `json:"mode"`
	App           string `json:"app,omitempty"`
	Skill         string `json:"skill,omitempty"`
	IncludesApp   bool   `json:"includes_app"`
	IncludesSkill bool   `json:"includes_skill"`
}

func summarizePackageBundle(pkg AgentPackage) PackageBundle {
	spec := effectiveInstallSpec(pkg)
	if spec == nil {
		return PackageBundle{Mode: "none"}
	}

	bundle := PackageBundle{}
	if spec.App != nil {
		bundle.IncludesApp = true
		bundle.App = firstNonEmpty(spec.App.Plugin, pkg.DisplayName, pkg.Name, pkg.ID)
	}
	if spec.Skill != nil {
		bundle.IncludesSkill = true
		bundle.Skill = firstNonEmpty(spec.Skill.Name, pkg.Name, pkg.ID)
	}

	switch {
	case bundle.IncludesApp && bundle.IncludesSkill:
		bundle.Mode = "bundle"
	case bundle.IncludesApp:
		bundle.Mode = "app"
	case bundle.IncludesSkill:
		bundle.Mode = "skill"
	default:
		bundle.Mode = "none"
	}
	return bundle
}
