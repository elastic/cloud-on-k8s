// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconciler

import (
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var (
	log = logf.Log.WithName("generic-reconciler")
)

// Params is a parameter object for the ReconcileResources function
type Params struct {
	Client k8s.Client
	// Owner will be set as the controller reference
	Owner metav1.Object
	// Expected the expected state of the resource going into reconciliation.
	Expected runtime.Object
	// Reconciled will contain the final state of the resource after reconciliation containing the
	// unification of remote and expected state.
	Reconciled runtime.Object
	// NeedsUpdate returns true when the object to be reconciled has changes that are not persisted remotely.
	NeedsUpdate func() bool
	// NeedsRecreate returns true when the object to be reconciled needs to be deleted and re-created because it cannot be updated.
	NeedsRecreate func() bool
	// UpdateReconciled modifies the resource pointed to by Reconciled to reflect the state of Expected
	UpdateReconciled func()
	// PreCreate is called just before the creation of the resource.
	PreCreate func()
	// PreUpdate is called just before the update of the resource.
	PreUpdate func()
	// PostUpdate is called immediately after the resource is successfully updated.
	PostUpdate func()
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
	gvk, err := apiutil.GVKForObject(params.Expected, scheme.Scheme)
	if err != nil {
		return err
	}
	kind := gvk.Kind

	if params.Owner != nil {
		if err := controllerutil.SetControllerReference(params.Owner, metaObj, scheme.Scheme); err != nil {
			return err
		}
	}

	create := func() error {
		log.Info("Creating resource", "kind", kind, "namespace", namespace, "name", name)
		if params.PreCreate != nil {
			params.PreCreate()
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
	}

	// Check if already exists
	err = params.Client.Get(types.NamespacedName{Name: name, Namespace: namespace}, params.Reconciled)
	if err != nil && apierrors.IsNotFound(err) {
		return create()
	} else if err != nil {
		log.Error(err, fmt.Sprintf("Generic GET for %s %s/%s failed with error", kind, namespace, name))
		return fmt.Errorf("failed to get %s %s/%s: %w", kind, namespace, name, err)
	}

	if params.NeedsRecreate != nil && params.NeedsRecreate() {
		log.Info("Resource cannot be updated, hence will be deleted and then recreated", "kind", kind, "namespace", namespace, "name", name)
		log.Info("Deleting resource", "kind", kind, "namespace", namespace, "name", name)
		reconciledMeta, err := meta.Accessor(params.Reconciled)
		if err != nil {
			return err
		}
		// Using a precondition here to make sure we delete the version of the resource we intend to delete and
		// to avoid accidentally deleting a resource already recreated for example
		uidToDelete := reconciledMeta.GetUID()
		resourceVersionToDelete := reconciledMeta.GetResourceVersion()
		opt := client.Preconditions{
			UID:             &uidToDelete,
			ResourceVersion: &resourceVersionToDelete,
		}

		err = params.Client.Delete(params.Expected, opt)
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete %s %s/%s: %w", kind, namespace, name, err)
		}
		return create()
	}

	// Update if needed
	if params.NeedsUpdate() {
		log.Info("Updating resource", "kind", kind, "namespace", namespace, "name", name)
		if params.PreUpdate != nil {
			params.PreUpdate()
		}
		params.UpdateReconciled()
		err := params.Client.Update(params.Reconciled)
		if err != nil {
			return err
		}
		if params.PostUpdate != nil {
			params.PostUpdate()
		}
	}
	return nil
}
