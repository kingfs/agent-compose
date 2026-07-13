package model

type ResourceKind string

const (
	ResourceKindProject ResourceKind = "project"
	ResourceKindAgent   ResourceKind = "agent"
	ResourceKindRun     ResourceKind = "run"
	ResourceKindSandbox ResourceKind = "sandbox"
	ResourceKindImage   ResourceKind = "image"
	ResourceKindCache   ResourceKind = "cache"
	ResourceKindVolume  ResourceKind = "volume"
)

type ResourceMatchType string

const (
	ResourceMatchName     ResourceMatchType = "name"
	ResourceMatchID       ResourceMatchType = "id"
	ResourceMatchIDPrefix ResourceMatchType = "id_prefix"
)

type ResourceResolveOptions struct {
	Ref   string
	Kinds []ResourceKind
}

type ResolvedResource struct {
	Kind        ResourceKind
	MatchType   ResourceMatchType
	ID          string
	ShortID     string
	Name        string
	ProjectID   string
	ProjectName string
	InspectRef  string
}

type ResourceResolveResult struct {
	Resources []ResolvedResource
	Warnings  []string
}
