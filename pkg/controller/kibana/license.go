// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const (
	// backgroundTasksLicenseRejectMsg is returned by the webhook when spec.backgroundTasks is set without an Enterprise
	// license, and logged by the controller when it skips reconciliation for the same reason.
	backgroundTasksLicenseRejectMsg = "spec.backgroundTasks (Kibana background task isolation) requires an Enterprise license"
)

// backgroundTasksState describes the state of spec.backgroundTasks on a Kibana with respect to the Enterprise license.
// It names the situation observed, not the action to take: each caller (webhook, controller) decides what to do.
type backgroundTasksState int

const (
	// backgroundTasksDisabled: spec.backgroundTasks is not set. The Kibana never had background tasks, or a user
	// turned them off.
	backgroundTasksDisabled backgroundTasksState = iota
	// backgroundTasksLicensed: spec.backgroundTasks is set and a valid Enterprise license is installed.
	backgroundTasksLicensed
	// backgroundTasksNewlyEnabledUnlicensed: spec.backgroundTasks is set without a valid Enterprise license, and
	// background tasks were not enabled before: a new Kibana, or an existing Kibana that turns them on.
	backgroundTasksNewlyEnabledUnlicensed
	// backgroundTasksAlreadyEnabledUnlicensed: spec.backgroundTasks is set without a valid Enterprise license, and
	// background tasks were already enabled: the existing pool outlives its license.
	backgroundTasksAlreadyEnabledUnlicensed
)

// backgroundTasksStateOf determines the backgroundTasksState of a Kibana. It is shared by the validating
// webhook and the controller. Neither relies on the previous spec: whether background tasks were already enabled is
// read from the cluster state, by fetching the background tasks Deployment of this Kibana. The Deployment is only
// fetched when background tasks are configured and Enterprise features are disabled.
// Errors from the license checker or from the Deployment lookup are returned as is: each caller decides whether to
// fail open or closed.
func backgroundTasksStateOf(ctx context.Context, c k8s.Client, kb *kbv1.Kibana, checker commonlicense.Checker) (backgroundTasksState, error) {
	if !kb.BackgroundTasksEnabled() {
		return backgroundTasksDisabled, nil
	}
	enabled, err := checker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return backgroundTasksDisabled, err
	}
	if enabled {
		return backgroundTasksLicensed, nil
	}
	alreadyEnabled, err := backgroundTasksDeploymentExists(ctx, c, kb)
	if err != nil {
		return backgroundTasksDisabled, err
	}
	if alreadyEnabled {
		return backgroundTasksAlreadyEnabledUnlicensed, nil
	}
	return backgroundTasksNewlyEnabledUnlicensed, nil
}

// backgroundTasksDeploymentExists reports whether the background tasks Deployment of the given Kibana exists.
func backgroundTasksDeploymentExists(ctx context.Context, c k8s.Client, kb *kbv1.Kibana) (bool, error) {
	var existing appsv1.Deployment
	err := c.Get(ctx, types.NamespacedName{Namespace: kb.Namespace, Name: kbv1.BackgroundTasksDeployment(kb.Name)}, &existing)
	switch {
	case err == nil:
		return true, nil
	case apierrors.IsNotFound(err):
		return false, nil
	default:
		return false, err
	}
}
