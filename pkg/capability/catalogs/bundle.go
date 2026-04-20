package agentstore

type PackageBundle struct {
	Mode          string `json:"mode"`
	Skill         string `json:"skill,omitempty"`
	IncludesSkill bool   `json:"includes_skill"`
}

func summarizePackageBundle(pkg AgentPackage) PackageBundle {
	spec := effectiveInstallSpec(pkg)
	if spec == nil {
		return PackageBundle{Mode: "none"}
	}

	bundle := PackageBundle{}
	if spec.Skill != nil {
		bundle.IncludesSkill = true
		bundle.Skill = firstNonEmpty(spec.Skill.Name, pkg.Name, pkg.ID)
	}

	switch {
	case bundle.IncludesSkill:
		bundle.Mode = "skill"
	default:
		bundle.Mode = "none"
	}
	return bundle
}
