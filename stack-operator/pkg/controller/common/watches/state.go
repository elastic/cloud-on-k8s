package watches

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
)

// NewDynamicWatches creates an initialized DynamicWatches container.
func NewDynamicWatches() DynamicWatches {
	return DynamicWatches{
		Secrets:        NewDynamicEnqueueRequest(),
		ClusterLicense: NewDynamicEnqueueRequest(),
	}
}

// DynamicWatches contains stateful dynamic watches. Intended as facility to pass around stateful dynamic watches and
// give each of them an identity.
type DynamicWatches struct {
	Secrets        *DynamicEnqueueRequest
	ClusterLicense *DynamicEnqueueRequest
}

// InjectScheme is used by the ControllerManager to inject Scheme into Sources, EventHandlers, Predicates, and
// Reconciles
func (w DynamicWatches) InjectScheme(scheme *runtime.Scheme) error {
	w.Secrets.InjectScheme(scheme)
	return nil
}

// DynamicWatches implements inject.Scheme mostly to facilitate testing. In production code injection happens on
// the individual watch level.
var _ inject.Scheme = DynamicWatches{}
