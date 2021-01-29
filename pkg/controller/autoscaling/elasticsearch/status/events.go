// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package status

import (
	"strings"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

// EmitEvents emits a selected type of event on the Kubernetes cluster event channel.
func EmitEvents(elasticsearch esv1.Elasticsearch, recorder record.EventRecorder, status Status) {
	for _, status := range status.AutoscalingPolicyStatuses {
		emitEventForAutoscalingPolicy(elasticsearch, recorder, status)
	}
}

func emitEventForAutoscalingPolicy(elasticsearch esv1.Elasticsearch, recorder record.EventRecorder, status AutoscalingPolicyStatus) {
	for _, event := range status.PolicyStates {
		switch event.Type {
		case VerticalScalingLimitReached, HorizontalScalingLimitReached, MemoryRequired, StorageRequired:
			recorder.Event(&elasticsearch, corev1.EventTypeWarning, string(event.Type), strings.Join(event.Messages, ". "))
		}
	}
}
