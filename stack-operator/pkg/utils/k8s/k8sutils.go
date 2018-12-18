package k8s

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ToObjectMeta returns an ObjectMeta based on the given NamespacedName
func ToObjectMeta(namespacedName types.NamespacedName) v1.ObjectMeta {
	return v1.ObjectMeta{
		Namespace: namespacedName.Namespace,
		Name:      namespacedName.Name,
	}
}

// ExtractNamespacedName returns an NamespacedName based on the given ObjectMeta
func ExtractNamespacedName(objectMeta v1.ObjectMeta) types.NamespacedName {
	return types.NamespacedName{
		Namespace: objectMeta.Namespace,
		Name:      objectMeta.Name,
	}
}
