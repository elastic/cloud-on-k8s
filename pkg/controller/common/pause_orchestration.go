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

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const (
	// PauseOrchestrationAnnotation pauses spec-driven orchestration (rolling upgrades, StatefulSet spec changes, scale
	// up/down) while keeping housekeeping running (certificate rotation, unicast hosts, user/secret reconciliation,
	// health monitoring).
	PauseOrchestrationAnnotation = "eck.k8s.elastic.co/pause-orchestration"
)

// IsOrchestrationPaused returns true if the PauseOrchestrationAnnotation exists and is set to true on the given resource
// when non-critical orchestration steps should be skipped.
func IsOrchestrationPaused(object metav1.Object) bool {
	return object.GetAnnotations()[PauseOrchestrationAnnotation] == "true"
}

// HasPendingChanges returns true if the given expected Object would result in an update to the existing cluster resource.
func HasPendingChanges(ctx context.Context, c k8s.Client, expected client.Object) (bool, error) {
	var existing client.Object
	if err := c.Get(ctx, k8s.ExtractNamespacedName(expected), existing); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return hash.GetTemplateHashLabel(existing.GetLabels()) != hash.GetTemplateHashLabel(expected.GetLabels()), nil
}

// SetPausedConditionAndEmitEvent adds the OrchestrationPaused condition with a value of True and emits an event.
func SetPausedConditionAndEmitEvent(
	ctx context.Context,
	client k8s.Client,
	recorder toolsevents.EventRecorder,
	object client.Object,
	expected client.Object,
	conditions *commonv1.Conditions,
) error {
	hasPending, err := HasPendingChanges(ctx, client, expected)
	if err != nil {
		return err
	}
	msg := "Orchestration is paused"
	if hasPending {
		msg = "Orchestration is paused, spec changes are pending"
	}

	*conditions = conditions.MergeWith(commonv1.Condition{
		Type:               commonv1.OrchestrationPaused,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Message:            msg,
	})
	k8s.EmitEvent(recorder, object, corev1.EventTypeWarning,
		events.EventReasonPaused, events.EventActionReconciliation, msg)
	return nil
}

// MaybeResetPausedCondition updates the OrchestrationPaused condition to False if it exists, or is a no-op if it does not
// already exist.
func MaybeResetPausedCondition(conditions *commonv1.Conditions) {
	orchestrationPausedIndex := conditions.Index(commonv1.OrchestrationPaused)
	if orchestrationPausedIndex >= 0 {
		*conditions = conditions.MergeWith(commonv1.Condition{
			Type:               commonv1.OrchestrationPaused,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Message:            "Orchestration has resumed normally",
		})
	}
}
