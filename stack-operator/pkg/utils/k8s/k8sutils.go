package k8s

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// DefaultNamespace is k8s default namespace
	DefaultNamespace = "default"
)

// NamespacedName builds a NamespacedName from the given args
func NamespacedName(namespace string, name string) types.NamespacedName {
	return types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
}

// ObjectMeta builds an ObjectMeta from the given args
func ObjectMeta(namespace string, name string) v1.ObjectMeta {
	return v1.ObjectMeta{
		Namespace: namespace,
		Name:      name,
	}
}

// ToObjectMeta returns an ObjectMeta based on the given NamespacedName
func ToObjectMeta(namespacedName types.NamespacedName) v1.ObjectMeta {
	return ObjectMeta(namespacedName.Namespace, namespacedName.Name)
}

// ToNamespacedName returns an NamespacedName based on the given ObjectMeta
func ToNamespacedName(objectMeta v1.ObjectMeta) types.NamespacedName {
	return NamespacedName(objectMeta.Namespace, objectMeta.Name)
}
