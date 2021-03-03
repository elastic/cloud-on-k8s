// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"context"
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// ConfigRefWatchName returns the name of the watch registered on the secret referenced in `configRef`.
func ConfigRefWatchName(resource types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-configref", resource.Namespace, resource.Name)
}

// ParseConfigRef retrieves the content of a secret referenced in `configRef`, sets up dynamic watches for that secret,
// and parses the secret content into a CanonicalConfig.
func ParseConfigRef(
	driver driver.Interface,
	resource runtime.Object, // eg. Beat, EnterpriseSearch
	configRef *commonv1.ConfigSource,
	secretKey string, // retrieve config data from that entry in the secret
) (*settings.CanonicalConfig, error) {
	resourceMeta, err := meta.Accessor(resource)
	if err != nil {
		return nil, err
	}
	namespace := resourceMeta.GetNamespace()
	resourceNsn := types.NamespacedName{Namespace: namespace, Name: resourceMeta.GetName()}

	// ensure watches match the referenced secret
	var secretNames []string
	if configRef != nil && configRef.SecretName != "" {
		secretNames = append(secretNames, configRef.SecretName)
	}
	if err := watches.WatchUserProvidedSecrets(resourceNsn, driver.DynamicWatches(), ConfigRefWatchName(resourceNsn), secretNames); err != nil {
		return nil, err
	}

	if len(secretNames) == 0 {
		// no secret referenced, nothing to do
		return nil, nil
	}

	var secret corev1.Secret
	if err := driver.K8sClient().Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: configRef.SecretName}, &secret); err != nil {
		// the secret may not exist (yet) in the cache, let's explicitly error out and retry later
		return nil, err
	}
	data, exists := secret.Data[secretKey]
	if !exists {
		msg := fmt.Sprintf("unable to parse configRef secret %s/%s: missing key %s", namespace, configRef.SecretName, secretKey)
		driver.Recorder().Event(resource, corev1.EventTypeWarning, events.EventReasonUnexpected, msg)
		return nil, errors.New(msg)
	}
	parsed, err := settings.ParseConfig(data)
	if err != nil {
		msg := fmt.Sprintf("unable to parse %s in configRef secret %s/%s", secretKey, namespace, configRef.SecretName)
		driver.Recorder().Event(resource, corev1.EventTypeWarning, events.EventReasonUnexpected, msg)
		return nil, errors.Wrap(err, msg)
	}
	return parsed, nil
}
