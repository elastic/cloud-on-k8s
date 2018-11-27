package provider

const (
	// Name of our persistent volume provider implementation
	Name = "volumes.k8s.elastic.co/elastic-local"
	// NodeAffinityLabel is the key for the label applied on Persistent Volumes once mounted on a node
	NodeAffinityLabel = "volumes.k8s.elastic.co/node-affinity"
)
