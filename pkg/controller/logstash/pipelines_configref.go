// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"fmt"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/driver"
)

// PipelinesRefWatchName returns the name of the watch registered on the secret referenced in `configRef`.
func PipelinesRefWatchName(resource types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-pipelinesref", resource.Namespace, resource.Name)
}

// ParseConfigRef retrieves the content of a secret referenced in `configRef`, sets up dynamic watches for that secret,
// and parses the secret content into a PipelinesConfig.
func ParseConfigRef(
	driver driver.Interface,
	resource runtime.Object,
	configRef *commonv1.ConfigSource,
	secretKey string, // retrieve config data from that entry in the secret
) (*PipelinesConfig, error) {
	parsed, err := common.ParseConfigRefToConfig(driver, resource, configRef, secretKey, PipelinesRefWatchName, Options)
	if err != nil {
		return nil, err
	}

	if parsed != nil {
		if err := checkIsArray(parsed); err != nil {
			resourceMeta, _ := meta.Accessor(resource)
			namespace := resourceMeta.GetNamespace()

			msg := fmt.Sprintf("unable to parse %s in configRef secret %s/%s", secretKey, namespace, configRef.SecretName)
			driver.Recorder().Event(resource, corev1.EventTypeWarning, events.EventReasonUnexpected, msg)
			return nil, errors.Wrap(err, msg)
		}
	}

	return (*PipelinesConfig)(parsed), nil
}
