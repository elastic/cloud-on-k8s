// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package defaults

import (
	v1 "k8s.io/api/core/v1"
)

// SetServiceDefaults updates the service with the provided defaults if they are not already set.
func SetServiceDefaults(
	svc *v1.Service,
	defaultLabels map[string]string,
	defaultSelector map[string]string,
	defaultPorts []v1.ServicePort,
) *v1.Service {
	if svc.ObjectMeta.Labels == nil {
		svc.ObjectMeta.Labels = defaultLabels
	} else {
		// add our labels, but don't overwrite user labels
		for k, v := range defaultLabels {
			if _, ok := svc.ObjectMeta.Labels[k]; !ok {
				svc.ObjectMeta.Labels[k] = v
			}
		}
	}

	if svc.Spec.Selector == nil {
		svc.Spec.Selector = defaultSelector
	}

	if svc.Spec.Ports == nil {
		svc.Spec.Ports = defaultPorts
	}

	return svc
}
