package workflow

import "context"

type CandidateKind string

const (
	CandidateDirect     CandidateKind = "direct"
	CandidateWorkflow   CandidateKind = "workflow"
	CandidateSpecialist CandidateKind = "specialist"
	CandidateToolchain  CandidateKind = "toolchain"
	CandidateClarify    CandidateKind = "clarify"
	CandidateApproval   CandidateKind = "approval"
)

type CandidateRequest struct {
	ID         string
	Input      string
	Title      string
	UserID     string
	Org        string
	Project    string
	Workspace  string
	SessionID  string
	ConfigPath string
}

type Candidate struct {
	Kind             CandidateKind
	Path             string
	ID               string
	Plugin           string
	Workflow         string
	App              string
	Confidence       float64
	RequiresApproval bool
	RiskLevel        string
	Reason           string
}

type PlannerClient interface{}

type LLMRouteDecision struct {
	Provider string
	Model    string
	Reason   string
}

type Router struct{}

func NewRouter(_ any, _ PlannerClient) *Router {
	return &Router{}
}

type Service struct {
	router *Router
}

func NewService(router *Router) *Service {
	return &Service{router: router}
}

func NewServiceForRegistry(_ any, _ PlannerClient) *Service {
	return NewService(NewRouter(nil, nil))
}

func (s *Service) Candidates(_ context.Context, _ CandidateRequest) ([]Candidate, error) {
	if s == nil {
		return nil, nil
	}
	return nil, nil
}

func BuildGuidance(_ ...any) string {
	return ""
}

func AppendSuggestedSummary(summary string, _ ...any) string {
	return summary
}

func DecideLLM(_ any, _ string) LLMRouteDecision {
	return LLMRouteDecision{}
}
