// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconciler

import (
	"fmt"
	"reflect"

	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("generic-reconciler")
)

// Params is a parameter object for the ReconcileResources function
type Params struct {
	Client k8s.Client
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
	// OnCreate allows for side-effects (logging) when a new resource will be created.
	OnCreate func()
	// OnUpdate allows for side-effects (logging) when a resources will be updated.
	OnUpdate func()
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
	err = params.Client.Get(types.NamespacedName{Name: name, Namespace: namespace}, params.Reconciled)
	if err != nil && apierrors.IsNotFound(err) {
		// Create if needed
		log.Info(fmt.Sprintf("Creating %s %s/%s", kind, namespace, name))
		if params.OnCreate != nil {
			params.OnCreate()
		}

		// Copy the content of params.Expected into params.Reconciled.
		// Unfortunately it's not straightforward to change the value of an interface underlying pointer,
		// so we need a small bit of reflection here.
		// This will panic if params.Expected and params.Reconciled don't have the same underlying type.
		expectedCopyValue := reflect.ValueOf(params.Expected.DeepCopyObject()).Elem()
		reflect.ValueOf(params.Reconciled).Elem().Set(expectedCopyValue)
		// Create the object, which modifies params.Reconciled in-place
		err = params.Client.Create(params.Reconciled)
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
		if params.OnUpdate != nil {
			params.OnUpdate()
		}
		params.UpdateReconciled()
		err := params.Client.Update(params.Reconciled)
		if err != nil {
			return err
		}
	}
	return nil
}
