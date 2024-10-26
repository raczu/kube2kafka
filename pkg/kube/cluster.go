package kube

type Cluster struct {
	// Name is the name of the cluster.
	Name string
	// TargetNamespace is the namespace to watch for events.
	TargetNamespace string
}
