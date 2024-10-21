// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package reconciler

import (
	"context"
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

// Params is a parameter object for the ReconcileResources function
type Params struct {
	// Context to be used in API requests
	Context context.Context
	// Client k8s client to use
	Client k8s.Client
	// Owner will be set as the controller reference
	Owner client.Object
	// Expected the expected state of the resource going into reconciliation.
	Expected client.Object
	// Reconciled will contain the final state of the resource after reconciliation containing the
	// unification of remote and expected state.
	Reconciled client.Object
	// NeedsUpdate returns true when the object to be reconciled has changes that are not persisted remotely.
	NeedsUpdate func() bool
	// NeedsRecreate returns true when the object to be reconciled needs to be deleted and re-created because it cannot be updated.
	NeedsRecreate func() bool
	// UpdateReconciled modifies the resource pointed to by Reconciled to reflect the state of Expected
	UpdateReconciled func()
	// PreCreate is called just before the creation of the resource.
	PreCreate func() error
	// PreUpdate is called just before the update of the resource.
	PreUpdate func() error
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

	gvk, err := apiutil.GVKForObject(params.Expected, scheme.Scheme)
	if err != nil {
		return err
	}

	if params.Owner != nil {
		if err := controllerutil.SetControllerReference(params.Owner, params.Expected, scheme.Scheme); err != nil {
			return err
		}
	}

	kind := gvk.Kind
	namespace := params.Expected.GetNamespace()
	name := params.Expected.GetName()
	log := ulog.FromContext(params.Context).WithValues("kind", kind, "namespace", namespace, "name", name)

	create := func() error {
		log.Info("Creating resource")
		if params.PreCreate != nil {
			if err := params.PreCreate(); err != nil {
				return err
			}
		}

		// Copy the content of params.Expected into params.Reconciled.
		// Unfortunately it's not straightforward to change the value of an interface underlying pointer,
		// so we need a small bit of reflection here.
		// This will panic if params.Expected and params.Reconciled don't have the same underlying type.
		expectedCopyValue := reflect.ValueOf(params.Expected.DeepCopyObject()).Elem()
		reflect.ValueOf(params.Reconciled).Elem().Set(expectedCopyValue)
		// Create the object, which modifies params.Reconciled in-place
		err = params.Client.Create(params.Context, params.Reconciled)
		if err != nil {
			return err
		}
		log.Info("Created resource successfully")
		return nil
	}

	// Check if already exists
	err = params.Client.Get(params.Context, types.NamespacedName{Name: name, Namespace: namespace}, params.Reconciled)
	if err != nil && apierrors.IsNotFound(err) {
		return create()
	} else if err != nil {
		log.Error(err, fmt.Sprintf("Generic GET for %s %s/%s failed with error", kind, namespace, name))
		return fmt.Errorf("failed to get %s %s/%s: %w", kind, namespace, name, err)
	}

	if params.NeedsRecreate != nil && params.NeedsRecreate() {
		log.Info("Deleting resource as it cannot be updated, it will be recreated")
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

		err = params.Client.Delete(params.Context, params.Expected, opt)
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete %s %s/%s: %w", kind, namespace, name, err)
		}
		log.Info("Deleted resource successfully")
		return create()
	}

	//nolint:nestif
	// Update if needed
	if params.NeedsUpdate() {
		log.Info("Updating resource")
		if params.PreUpdate != nil {
			if err := params.PreUpdate(); err != nil {
				return err
			}
		}
		reconciledMeta, err := meta.Accessor(params.Reconciled)
		if err != nil {
			return err
		}

		// retain the resource version to avoid unconditional updates
		resourceVersion := reconciledMeta.GetResourceVersion()
		params.UpdateReconciled()
		// and set the resource version back into the resource to indicate the state we are basing the update off of
		reconciledMeta.SetResourceVersion(resourceVersion)
		// also keep the owner references up to date
		expectedMeta, err := meta.Accessor(params.Expected)
		if err != nil {
			return err
		}
		expectedOwners := expectedMeta.GetOwnerReferences()
		if expectedOwners != nil {
			// we can safely assume we have just one reference here given that it was created just above
			// but we don't want to replace wholesale in case a user has set an additional reference
			k8s.OverrideControllerReference(reconciledMeta, expectedOwners[0])
		}

		err = params.Client.Update(params.Context, params.Reconciled)
		if err != nil {
			return err
		}
		if params.PostUpdate != nil {
			params.PostUpdate()
		}
		log.Info("Updated resource successfully")
	}
	return nil
}
