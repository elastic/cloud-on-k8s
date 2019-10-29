// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
)

// MinVersion extracts the currently running Elasticsearch versions from the running pods
func minVersion(pods []corev1.Pod) (*version.Version, error) {
	var vs []version.Version
	for _, pod := range pods {
		v, err := label.ExtractVersion(pod.Labels)
		if err != nil {
			return nil, err
		}
		vs = append(vs, *v)
	}
	return version.Min(vs), nil
}

func (d *defaultDriver) verifySupportsExistingPods(pods []corev1.Pod) error {
	for _, pod := range pods {
		v, err := label.ExtractVersion(pod.Labels)
		if err != nil {
			return err
		}
		if err := d.SupportedVersions.Supports(*v); err != nil {
			return errors.Wrapf(err, "%s has incompatible version", pod.Name)
		}
	}
	return nil
}
