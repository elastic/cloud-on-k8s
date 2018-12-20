package reconciler

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("generic-reconciler")
)

// Params is a parameter object for the ReconcileResources function
type Params struct {
	Client client.Client
	// Scheme with all custom resources kinds registered.
	Scheme *runtime.Scheme
	// Owner will be set as the controller reference
	Owner metav1.Object
	// Expected the expected state of the resource going into reconciliation.
	Expected runtime.Object
	// Reconciled will contain the final state of the resource after reconciliation containing the
	// unification of remote and expected state.
	Reconciled runtime.Object
	// NeedsUpdate returns true when the object to be reconciled has changes that are not persisted remotely.
	NeedsUpdate func() bool
	// UpdateReconciled modifies the resource pointed to by Reconciled to reflect the state of Expected
	UpdateReconciled func()
}

func (p Params) CheckNilValues() error {
	if p.Reconciled == nil {
		return errors.New("Reconciled must not be nil")
	}
	if p.UpdateReconciled == nil {
		return errors.New("UpdateReconciled must not be nil")
	}
	if p.NeedsUpdate == nil {
		return errors.New("NeedsUpdate must not be nil")
	}
	if p.Expected == nil {
		return errors.New("Expected must not be nil")
	}
	return nil

}

// ReconcileResource is a generic reconciliation function for resources that need to
// implement runtime.Object and meta/v1.Object.
func ReconcileResource(params Params) error {
	err := params.CheckNilValues()
	if err != nil {
		return err
	}
	metaObj, err := meta.Accessor(params.Expected)
	if err != nil {
		return err
	}
	namespace := metaObj.GetNamespace()
	name := metaObj.GetName()
	kind := params.Expected.GetObjectKind().GroupVersionKind().Kind

	if err := controllerutil.SetControllerReference(params.Owner, metaObj, params.Scheme); err != nil {
		return err
	}

	// Check if already exists
	err = params.Client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, params.Reconciled)
	if err != nil && apierrors.IsNotFound(err) {
		// Create if needed
		log.Info(fmt.Sprintf("Creating %s %s/%s", kind, namespace, name))

		err = params.Client.Create(context.TODO(), params.Expected)
		if err != nil {
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, fmt.Sprintf("Generic GET for %s %s/%s failed with error", kind, namespace, name))
		return err
	}

	// Update if needed
	if params.NeedsUpdate() {
		log.Info(fmt.Sprintf("Updating %s %s/%s ", kind, namespace, name))
		params.UpdateReconciled()
		err := params.Client.Update(context.TODO(), params.Reconciled)
		if err != nil {
			return err
		}
	}
	return nil
}
