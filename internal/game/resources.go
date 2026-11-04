package game

// ResourceKind separates unrelated native namespaces on the same map.
type ResourceKind byte

const (
	ResourceCollectionActor ResourceKind = 1
	ResourceGroundItem      ResourceKind = 2
	ResourceCollectionArea  ResourceKind = 3
)
