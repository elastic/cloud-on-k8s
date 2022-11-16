// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package status

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
)

// EmitEvents emits a selected type of event on the Kubernetes cluster event channel.
func EmitEvents(elasticsearch esv1.Elasticsearch, recorder record.EventRecorder, status v1alpha1.ElasticsearchAutoscalerStatus) {
	for _, status := range status.AutoscalingPolicyStatuses {
		emitEventForAutoscalingPolicy(elasticsearch, recorder, status)
	}
}

func emitEventForAutoscalingPolicy(elasticsearch esv1.Elasticsearch, recorder record.EventRecorder, status v1alpha1.AutoscalingPolicyStatus) {
	for _, event := range status.PolicyStates {
		//nolint:exhaustive
		switch event.Type {
		case v1alpha1.VerticalScalingLimitReached, v1alpha1.HorizontalScalingLimitReached, v1alpha1.MemoryRequired, v1alpha1.StorageRequired, v1alpha1.UnexpectedNodeStorageCapacity:
			recorder.Event(&elasticsearch, corev1.EventTypeWarning, string(event.Type), strings.Join(event.Messages, ". "))
		}
	}
}
