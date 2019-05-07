// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	corev1 "k8s.io/api/core/v1"
)

// MinVersion extracts the currently running Elasticsearch versions from the running pods
func MinVersion(pods []corev1.Pod) (*version.Version, error) {
	var vs []version.Version
	for _, pod := range pods {
		v, err := label.ExtractVersion(pod)
		if err != nil {
			return nil, err
		}
		vs = append(vs, *v)
	}
	return version.Min(vs), nil
}
