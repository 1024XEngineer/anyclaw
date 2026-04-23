package sdk

type Status struct {
	OK         bool   `json:"ok"`
	Status     string `json:"status"`
	Version    string `json:"version"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Address    string `json:"address"`
	StartedAt  string `json:"started_at,omitempty"`
	WorkingDir string `json:"working_dir"`
	WorkDir    string `json:"work_dir"`
	Sessions   int    `json:"sessions"`
	Events     int    `json:"events"`
	Skills     int    `json:"skills"`
	Tools      int    `json:"tools"`
	Secured    bool   `json:"secured"`
	Users      int    `json:"users"`
}
