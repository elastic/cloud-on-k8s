package common

import (
	deploymentsv1alpha1 "github.com/elastic/k8s-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/k8s-operators/stack-operator/pkg/utils/stringsutil"
)

// StackID returns the qualified identifier for this stack deployment
// based on the given namespace and stack name, following
// the convention: <namespace>-<stack name>
func StackID(s deploymentsv1alpha1.Stack) string {
	return stringsutil.Concat(s.Namespace, "-", s.Name)
}
