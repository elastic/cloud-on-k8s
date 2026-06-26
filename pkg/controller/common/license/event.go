// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"os"

	corev1 "k8s.io/api/core/v1"
	toolsevents "k8s.io/client-go/tools/events"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// EmitEnterpriseFeatureEvent emits a Warning event on the operator's own Pod
// when an enterprise feature is used without a valid enterprise license.
// The pod name is resolved via os.Hostname(), which Kubernetes always sets to
// the pod name. If the hostname cannot be determined the call is a no-op.
func EmitEnterpriseFeatureEvent(recorder toolsevents.EventRecorder, operatorNS, msg string) {
	podName, err := os.Hostname()
	if err != nil || podName == "" {
		return
	}
	pod := corev1.Pod{}
	pod.APIVersion = "v1"
	pod.Kind = "Pod"
	pod.Name = podName
	pod.Namespace = operatorNS
	k8s.EmitEvent(recorder, &pod, corev1.EventTypeWarning, EventInvalidLicense, events.EventActionLicenseCheck, msg)
}
