package k8s

import (
	"io/ioutil"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// InClusterNamespacePath is the path to the file containing the current namespace
	// this pod is running in (when running in K8s).
	InClusterNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

// ToObjectMeta returns an ObjectMeta based on the given NamespacedName
func ToObjectMeta(namespacedName types.NamespacedName) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Namespace: namespacedName.Namespace,
		Name:      namespacedName.Name,
	}
}

// ExtractNamespacedName returns an NamespacedName based on the given ObjectMeta
func ExtractNamespacedName(objectMeta metav1.ObjectMeta) types.NamespacedName {
	return types.NamespacedName{
		Namespace: objectMeta.Namespace,
		Name:      objectMeta.Name,
	}
}

// GuessCurrentNamespace tries to retrieve the current namespace
// this program might be running in, by checking the filesystem.
// In case of error, or if not running within a K8s cluster,
// it returns the provided fallback namespace.
func GuessCurrentNamespace(fallback string) string {
	namespaceName, err := ioutil.ReadFile(InClusterNamespacePath)
	if err != nil {
		return fallback
	}
	return strings.TrimSpace(string(namespaceName))
}
