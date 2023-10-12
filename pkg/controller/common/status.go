// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

// DeploymentStatus returns a DeploymentStatus computed from the given args.
// Unknown fields are inherited from current.
func DeploymentStatus(ctx context.Context, current commonv1.DeploymentStatus, dep appsv1.Deployment, pods []corev1.Pod, versionLabel string) (commonv1.DeploymentStatus, error) {
	status := *current.DeepCopy()
	if dep.Spec.Selector != nil {
		selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
		if err != nil {
			return commonv1.DeploymentStatus{}, err
		}
		status.Selector = selector.String()
	}
	status.Count = dep.Status.Replicas
	status.AvailableNodes = dep.Status.AvailableReplicas
	status.Version = LowestVersionFromPods(ctx, status.Version, pods, versionLabel)
	status.Health = commonv1.RedHealth
	for _, c := range dep.Status.Conditions {
		if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
			status.Health = commonv1.GreenHealth
		}
	}
	return status, nil
}

// LowestVersionFromPods parses versions from the given pods based on the given label,
// and returns the lowest one.
func LowestVersionFromPods(ctx context.Context, currentVersion string, pods []corev1.Pod, versionLabel string) string {
	lowestVersion, err := version.MinInPods(pods, versionLabel)
	if err != nil {
		ulog.FromContext(ctx).Error(err, "failed to parse version from Pods", "version_label", versionLabel)
		return currentVersion
	}
	if lowestVersion == nil {
		return currentVersion
	}
	return lowestVersion.String()
}

// UpdateStatus updates the status sub-resource of the given object.
func UpdateStatus(ctx context.Context, client k8s.Client, obj client.Object) error {
	return client.Status().Update(ctx, obj)
}
