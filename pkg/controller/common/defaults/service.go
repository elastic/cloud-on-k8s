// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package defaults

import (
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
	v1 "k8s.io/api/core/v1"
)

// SetServiceDefaults updates the service with the provided defaults if they are not already set.
func SetServiceDefaults(
	svc *v1.Service,
	defaultLabels map[string]string,
	defaultSelector map[string]string,
	defaultPorts []v1.ServicePort,
) *v1.Service {
	svc.Labels = maps.MergePreservingExistingKeys(svc.Labels, defaultLabels)

	if svc.Spec.Selector == nil {
		svc.Spec.Selector = defaultSelector
	}

	if svc.Spec.Ports == nil {
		svc.Spec.Ports = defaultPorts
	}

	return svc
}
