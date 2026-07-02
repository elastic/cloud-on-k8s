// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/enterprisesearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	maps "github.com/elastic/cloud-on-k8s/v3/pkg/apis/maps/v1alpha1"
	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const (
	// PauseOrchestrationAnnotation is re-exported from commonv1 so existing callers keep working; the canonical
	// definition lives in pkg/apis/common/v1 because the validating webhook depends on it.
	PauseOrchestrationAnnotation = commonv1.PauseOrchestrationAnnotation
	// PausedWithPendingChangesMessage is the message displayed in the OrchestrationPaused condition when the
	// PauseOrchestrationAnnotation is enabled and spec changes have been made.
	PausedWithPendingChangesMessage = "Orchestration paused via annotation; spec changes are pending and will be applied on resume"
	// PausedNoChangesMessage is the message displayed in the OrchestrationPaused condition when the
	// PauseOrchestrationAnnotation is enabled but no spec changes have been made.
	PausedNoChangesMessage = "Orchestration paused via annotation; no pending spec changes detected"
	// PausedWaitingMessage is the message displayed in the OrchestrationPaused condition when the
	// PauseOrchestrationAnnotation is enabled but the resource pods have not stabilized.
	PausedWaitingMessage = "Orchestration paused via annotation; waiting for pods to stabilize"
	// PausedOrchestrationResumed is the message displayed in the OrchestrationPaused condition when the
	// PauseOrchestrationAnnotation annotation has been disabled after previously being enabled. If the
	// PauseOrchestrationAnnotation has never been set, the OrchestrationPaused condition should not exist at all.
	PausedOrchestrationResumed = "Orchestration has resumed normally"
)

// IsOrchestrationPaused returns true if the PauseOrchestrationAnnotation exists and is set to true on the given resource
// when non-critical orchestration steps should be skipped.
func IsOrchestrationPaused(object metav1.Object) bool {
	return object.GetAnnotations()[PauseOrchestrationAnnotation] == "true"
}

// setPausedConditionAndEmitEvent adds the OrchestrationPaused condition with a value of True on the parent, and emits a
// Warning event when spec changes are pending (no event is emitted on the no-changes path). The parent is the ECK CR,
// such as v1.Kibana, to be updated with the appropriate v1.Conditions, while the expected client.Object is the
// underlying kubernetes resource that is created as a result of the ECK object's Spec.
func setPausedConditionAndEmitEvent(
	recorder toolsevents.EventRecorder,
	parent ObjectWithConditions,
	expected client.Object,
	actual client.Object,
) {
	hasPending := HasPendingChanges(expected, actual)
	msg := PausedNoChangesMessage
	if hasPending {
		msg = PausedWithPendingChangesMessage
		k8s.EmitEvent(recorder, parent, corev1.EventTypeWarning,
			events.EventReasonPaused, events.EventActionReconciliation, msg)
	}

	parent.MergeConditions(commonv1.Condition{
		Type:               commonv1.OrchestrationPaused,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Message:            msg,
	})
}

// ReconcilePauseAware wraps the given reconcileFn with pause-orchestration handling: when paused, it fetches the
// existing resource, sets the OrchestrationPaused condition (with a Warning event if spec changes are pending), and
// returns the live object without applying any spec change. When not paused, it clears any stale OrchestrationPaused
// condition and delegates to reconcileFn.
//
// PT exists so the function can take the address of T values and pass them to client.Object-typed APIs (Get,
// setPausedConditionAndEmitEvent). It is constrained to be a pointer to T that also implements client.Object.
func ReconcilePauseAware[T any, PT interface {
	*T
	client.Object
}](
	ctx context.Context,
	c k8s.Client,
	recorder toolsevents.EventRecorder,
	expected T,
	owner ObjectWithConditions,
	reconcileFn func(context.Context, k8s.Client, T, client.Object) (T, error),
) (T, error) {
	if IsOrchestrationPaused(owner) {
		var actual T
		var actualForDiff client.Object // nil when the resource doesn't exist yet — treated as "pending changes" by HasPendingChanges
		err := c.Get(ctx, k8s.ExtractNamespacedName(PT(&expected)), PT(&actual))
		switch {
		case err == nil:
			actualForDiff = PT(&actual)
		case apierrors.IsNotFound(err):
			// first-reconcile-while-paused: leave actualForDiff nil so the condition is still reported with PendingChanges
		default:
			return *new(T), err
		}
		setPausedConditionAndEmitEvent(recorder, owner, PT(&expected), actualForDiff)
		return actual, nil
	}

	maybeResetPausedCondition(recorder, owner)

	return reconcileFn(ctx, c, expected, owner)
}

// maybeResetPausedCondition updates the OrchestrationPaused condition to False if it exists, or is a no-op if it does not
// already exist.
func maybeResetPausedCondition(
	recorder toolsevents.EventRecorder,
	parent ObjectWithConditions,
) {
	conditions := parent.Conditions()

	orchestrationPausedIndex := conditions.Index(commonv1.OrchestrationPaused)
	if orchestrationPausedIndex >= 0 && conditions[orchestrationPausedIndex].Status == corev1.ConditionTrue {
		parent.MergeConditions(commonv1.Condition{
			Type:               commonv1.OrchestrationPaused,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Message:            PausedOrchestrationResumed,
		})
		k8s.EmitEvent(recorder, parent, corev1.EventTypeNormal,
			events.EventReasonResumed, events.EventActionOrchestrationResumed, PausedOrchestrationResumed)
	}
}

// HasPendingChanges returns true if the given expected client.Object (Deployment, StatefulSet, or DaemonSet) would result in
// an update to the existing resource. This is predicated on the common.k8s.elastic.co/template-hash label being set on
// the expected client.Object.
func HasPendingChanges(expected client.Object, actual client.Object) bool {
	if actual == nil && expected != nil {
		return true
	}

	if actual == nil || expected == nil {
		return false
	}

	return hash.GetTemplateHashLabel(actual.GetLabels()) != hash.GetTemplateHashLabel(expected.GetLabels())
}

// ObjectWithConditions provides an interfacing wrapping a client.Object with an additional MergeCondition function to
// allow setPausedConditionAndEmitEvent to be agnostic of the underlying resource type. This is defined here because:
//  1. controller-gen does not allow the interface type to be defined in the API source, preventing this from being
//     defined alongside the commonv1.Conditions
//  2. it is only required by the pause-orchestration implementation
//  3. it enables the re-use of the setPausedConditionAndEmitEvent function between Deployment, DaemonSet, and
//     StatefulSet resource types.
type ObjectWithConditions interface {
	client.Object
	MergeConditions(conditions ...commonv1.Condition)
	Conditions() commonv1.Conditions
}

// These type-checks are defined here to avoid an import cycle between the API packages and this package, but also
// provides a clearly defined expectation as far as the implementors. Elasticsearch is notably and intentionally absent
// as its pause-orchestration handling is very specific to Elasticsearch as a result of the different node tiers.
var _ ObjectWithConditions = (*kbv1.Kibana)(nil)
var _ ObjectWithConditions = (*apmv1.ApmServer)(nil)
var _ ObjectWithConditions = (*v1beta1.Beat)(nil)
var _ ObjectWithConditions = (*eprv1alpha1.PackageRegistry)(nil)
var _ ObjectWithConditions = (*maps.ElasticMapsServer)(nil)
var _ ObjectWithConditions = (*agentv1alpha1.Agent)(nil)
var _ ObjectWithConditions = (*entv1.EnterpriseSearch)(nil)
var _ ObjectWithConditions = (*autoopsv1alpha1.AutoOpsAgentPolicy)(nil)
