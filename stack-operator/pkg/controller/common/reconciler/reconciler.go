package reconciler

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Owner  metav1.Object
}

func (r Reconciler) ReconcileObjWithEffect(
	obj runtime.Object,
	new func() runtime.Object,
	diff func(expected, found runtime.Object) bool,
	mod func(expected, found runtime.Object) runtime.Object,
	effect func(result runtime.Object)) error {
	meta, ok := obj.(metav1.Object)
	if !ok {
		return errors.Errorf("%v is not a metadata Object", obj)
	}
	namespace := meta.GetNamespace()
	name := meta.GetName()
	kinds, unversioned, err := r.Scheme.ObjectKinds(obj)
	kind := "unknown"
	if !unversioned && err == nil {
		kind = kinds[0].Kind
	}
	if err := controllerutil.SetControllerReference(r.Owner, meta, r.Scheme); err != nil {
		return err
	}
	// Check if already exists
	expected := obj
	found := new()
	err = r.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		// Create if needed
		log.Info(fmt.Sprintf("Creating %s %s/%s", kind, namespace, name))

		err = r.Create(context.TODO(), expected)
		if err != nil {
			return err
		}
		return nil
	} else if err != nil {
		return err
	}

	// Update if needed
	if diff(expected, found) {
		log.Info(fmt.Sprintf("Updating %s %s/%s ", kind, namespace, name))
		err := r.Update(context.TODO(), mod(expected, found))
		if err != nil {
			return err
		}
	}
	effect(found)
	return nil
}

// ReconcileObj reconciles a given runtime object using the factory, diffing and modifying functions given.
func (r Reconciler) ReconcileObj(
	obj runtime.Object,
	new func() runtime.Object,
	diff func(expected, found runtime.Object) bool,
	mod func(expected, found runtime.Object) runtime.Object,
) (runtime.Object, error) {
	result := obj
	err := r.ReconcileObjWithEffect(obj, new, diff, mod, func(res runtime.Object) {
		result = res
	})
	return result, err
}
