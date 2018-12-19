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
	client.Client
	Scheme *runtime.Scheme
	// Owner will be set as the controller reference
	Owner metav1.Object
	// Object the resource to reconcile
	Expected runtime.Object
	// Reconciled the resource after reconciliation
	Reconciled runtime.Object
	// NeedsUpdate returns true when the object to be reconciled has changes that are not persisted remotely.
	NeedsUpdate func() bool
	// UpdateReconciled modifies the resource pointed to by Reconciled to reflect the state of Expected
	UpdateReconciled func()
}

// ReconcileResource is a generic reconciliation function for resources that need to
// implement runtime.Object and meta/v1.Object.
func ReconcileResource(params Params) error {
	if params.Reconciled == nil {
		return errors.New("Reconciled must not be nil")
	}
	if params.UpdateReconciled == nil {
		return errors.New("UpdateReconciled must not be nil")
	}
	if params.NeedsUpdate == nil {
		return errors.New("NeedsUpdate must not be nil")
	}

	metaObj, err := meta.Accessor(params.Expected)
	if err != nil  {
		return err
	}
	namespace := metaObj.GetNamespace()
	name := metaObj.GetName()
	kinds, _, err := params.Scheme.ObjectKinds(params.Expected)
	if err != nil {
		return err
	}
	kind := kinds[0].Kind

	if err := controllerutil.SetControllerReference(params.Owner, metaObj, params.Scheme); err != nil {
		return err
	}

	// Check if already exists
	err = params.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, params.Reconciled)
	if err != nil && apierrors.IsNotFound(err) {
		// Create if needed
		log.Info(fmt.Sprintf("Creating %s %s/%s", kind, namespace, name))

		err = params.Create(context.TODO(), params.Expected)
		if err != nil {
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Generic GET failed with error")
		return err
	}

	// Update if needed
	if params.NeedsUpdate() {
		log.Info(fmt.Sprintf("Updating %s %s/%s ", kind, namespace, name))
		params.UpdateReconciled()
		err := params.Update(context.TODO(), params.Reconciled)
		if err != nil {
			return err
		}
	}
	return nil
}
