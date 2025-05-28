// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package defaults

import (
	v1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

// SetServiceDefaults updates the service with the provided defaults if they are not already set.
func SetServiceDefaults(
	svc *v1.Service,
	defaultMetadata metadata.Metadata,
	defaultSelector map[string]string,
	defaultPorts []v1.ServicePort,
) *v1.Service {
	svc.Labels = maps.MergePreservingExistingKeys(svc.Labels, defaultMetadata.Labels)
	svc.Annotations = maps.MergePreservingExistingKeys(svc.Annotations, defaultMetadata.Annotations)

	if svc.Spec.Selector == nil {
		svc.Spec.Selector = defaultSelector
	}

	if svc.Spec.Ports == nil {
		svc.Spec.Ports = defaultPorts
	}

	return svc
}
