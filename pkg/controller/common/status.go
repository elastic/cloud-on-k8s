// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"context"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeploymentStatus returns a DeploymentStatus computed from the given args.
// Unknown fields are inherited from current.
func DeploymentStatus(current commonv1.DeploymentStatus, dep appsv1.Deployment, pods []corev1.Pod, versionLabel string) commonv1.DeploymentStatus {
	status := *current.DeepCopy()
	status.AvailableNodes = dep.Status.AvailableReplicas
	status.Version = LowestVersionFromPods(status.Version, pods, versionLabel)
	status.Health = commonv1.RedHealth
	for _, c := range dep.Status.Conditions {
		if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
			status.Health = commonv1.GreenHealth
		}
	}
	return status
}

// LowestVersionFromPods parses versions from the given pods based on the given label,
// and returns the lowest one.
func LowestVersionFromPods(currentVersion string, pods []corev1.Pod, versionLabel string) string {
	lowestVersion, err := version.MinInPods(pods, versionLabel)
	if err != nil {
		log.Error(err, "failed to parse version from Pods", "version_label", versionLabel)
		return currentVersion
	}
	if lowestVersion == nil {
		return currentVersion
	}
	return lowestVersion.String()
}

// UpdateStatus updates the status sub-resource of the given object.
func UpdateStatus(client k8s.Client, obj client.Object) error {
	err := client.Status().Update(context.Background(), obj)
	return workaroundStatusUpdateError(err, client, obj)
}

// workaroundStatusUpdateError handles a bug on k8s < 1.15 that prevents status subresources updates
// to be performed if the target resource storedVersion does not match the given resource version
// (eg. storedVersion=v1beta1 vs. resource version=v1).
// This is fixed by https://github.com/kubernetes/kubernetes/pull/78713 in k8s 1.15.
// In case that happens here, let's retry the update on the full resource instead of the status subresource.
func workaroundStatusUpdateError(err error, client k8s.Client, obj client.Object) error {
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
	return client.Update(context.Background(), obj)
}
