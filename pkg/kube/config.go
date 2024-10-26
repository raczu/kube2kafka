package kube

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
)

// GetKubeConfig returns a Kubernetes configuration based on the provided path.
// If the path is empty, the function will try to use the in-cluster configuration or
// search for the kubeconfig in the default location.
func GetKubeConfig(path string) (*rest.Config, error) {
	if path != "" {
		return clientcmd.BuildConfigFromFlags("", path)
	}

	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return rest.InClusterConfig()
	}

	kubeconfig := os.Getenv(clientcmd.RecommendedConfigPathEnvVar)
	if kubeconfig == "" {
		kubeconfig = clientcmd.RecommendedHomeFile
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfig)

}
