// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
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
	// PauseOrchestrationAnnotation pauses spec-driven orchestration (rolling upgrades, StatefulSet spec changes, scale
	// up/down) while keeping housekeeping running (certificate rotation, unicast hosts, user/secret reconciliation,
	// health monitoring).
	PauseOrchestrationAnnotation = "eck.k8s.elastic.co/pause-orchestration"
	// PausedWithPendingChangesMessage is the message displayed in the OrchestrationPaused condition when the
	// PauseOrchestrationAnnotation is enabled and spec changes have been made.
	PausedWithPendingChangesMessage = "Orchestration paused via annotation; spec changes are pending and will be applied on resume"
	// PausedNoChangesMessage is the message displayed in the OrchestrationPaused condition when the
	// PauseOrchestrationAnnotation is enabled but no spec changes have been made.
	PausedNoChangesMessage = "Orchestration paused via annotation; no pending spec changes detected"
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

// SetPausedConditionAndEmitEvent adds the OrchestrationPaused condition with a value of True and emits an event. The parent
// is the ECK object, such as v1.Kibana, to be updated with the appropriate v1.Conditions, while the expected client.Object
// is the underlying kubernetes resource that is created as a result of the ECK object's Spec.
func SetPausedConditionAndEmitEvent(
	ctx context.Context,
	client k8s.Client,
	recorder toolsevents.EventRecorder,
	parent ObjectWithConditions,
	expected client.Object,
) error {
	hasPending, err := hasPendingChanges(ctx, client, expected)
	if err != nil {
		return err
	}
	msg := PausedNoChangesMessage
	if hasPending {
		msg = PausedWithPendingChangesMessage
	}

	parent.MergeConditions(commonv1.Condition{
		Type:               commonv1.OrchestrationPaused,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Message:            msg,
	})

	k8s.EmitEvent(recorder, parent, corev1.EventTypeWarning,
		events.EventReasonPaused, events.EventActionReconciliation, msg)
	return nil
}

// MaybeResetPausedCondition updates the OrchestrationPaused condition to False if it exists, or is a no-op if it does not
// already exist.
func MaybeResetPausedCondition(conditions *commonv1.Conditions) {
	if conditions == nil {
		return
	}

	orchestrationPausedIndex := conditions.Index(commonv1.OrchestrationPaused)
	if orchestrationPausedIndex >= 0 {
		*conditions = conditions.MergeWith(commonv1.Condition{
			Type:               commonv1.OrchestrationPaused,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Message:            PausedOrchestrationResumed,
		})
	}
}

// hasPendingChanges returns true if the given expected client.Object (Deployment, StatefulSet, or DaemonSet) would result in
// an update to the existing resource. This is predicated on the common.k8s.elastic.co/template-hash label being set on
// the expected client.Object.
func hasPendingChanges(ctx context.Context, c k8s.Client, expected client.Object) (bool, error) {
	existing, ok := reflect.New(reflect.TypeOf(expected).Elem()).Interface().(client.Object)
	if !ok {
		// This would obviously have been caught at compile time and would therefore never happen, but golangci-lint
		// requires checking that the cast was successful
		return false, fmt.Errorf("%T does not implement the client.Object interface", expected)
	}
	if err := c.Get(ctx, k8s.ExtractNamespacedName(expected), existing); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}

	return hash.GetTemplateHashLabel(existing.GetLabels()) != hash.GetTemplateHashLabel(expected.GetLabels()), nil
}

// ObjectWithConditions provides an interfacing wrapping a client.Object with an additional MergeCondition function to
// allow SetPausedConditionAndEmitEvent to be agnostic of the underlying resource type. This is defined here because:
//  1. controller-gen does not allow the interface type to be defined in the API source, preventing this from being
//     defined alongside the commonv1.Conditions
//  2. it is only required by the pause-orchestration implementation
//  3. it enables the re-use of the SetPausedConditionAndEmitEvent function between Deployment, DaemonSet, and
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
