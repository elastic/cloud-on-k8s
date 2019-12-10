// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// UpdateStatus updates the status sub-resource of the given object.
func UpdateStatus(client k8s.Client, obj runtime.Object) error {
	err := client.Status().Update(obj)
	return workaroundStatusUpdateError(err, client, obj)
}

// workaroundStatusUpdateError handles a bug on k8s < 1.15 that prevents status subresources updates
// to be performed if the target resource storedVersion does not match the given resource version
// (eg. storedVersion=v1beta1 vs. resource version=v1).
// This is fixed by https://github.com/kubernetes/kubernetes/pull/78713 in k8s 1.15.
// In case that happens here, let's retry the update on the full resource instead of the status subresource.
func workaroundStatusUpdateError(err error, client k8s.Client, obj runtime.Object) error {
	if !apierrors.IsInvalid(err) {
		// not the case we're looking for here
		return err
	}
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	log.Info(
		"Status sub-resource update failed, attempting to update the entire resource instead",
		"namespace", accessor.GetNamespace(),
		"name", accessor.GetName(),
	)
	return client.Update(obj)
}
