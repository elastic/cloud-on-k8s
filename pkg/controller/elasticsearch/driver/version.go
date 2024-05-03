// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
)

func (d *defaultDriver) verifySupportsExistingPods(pods []corev1.Pod) error {
	for _, pod := range pods {
		v, err := label.ExtractVersion(pod.Labels)
		if err != nil {
			return err
		}
		if err := d.SupportedVersions.WithinRange(v); err != nil {
			return errors.Wrapf(err, "%s has incompatible version", pod.Name)
		}
	}
	return nil
}
