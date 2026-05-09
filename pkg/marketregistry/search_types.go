package marketregistry

type searchCandidate struct {
	Artifact
	lexicalRank float64
}

const (
	defaultSearchLimit        = 50
	defaultSearchCandidateCap = 200
)
