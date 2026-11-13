// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pipelines

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
)

// SecretRefWatchName returns the name of the watch registered on the secret referenced in `pipelinesRef`.
func SecretRefWatchName(resource types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-pipelinesref", resource.Namespace, resource.Name)
}

// ConfigMapRefWatchName returns the name of the watch registered on the configmap referenced in `pipelinesRef`.
func ConfigMapRefWatchName(resource types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-pipelinesref-cm", resource.Namespace, resource.Name)
}

// ParsePipelinesRef retrieves pipeline definitions from a Secret or ConfigMap referenced in `pipelinesRef`,
// maintains dynamic watches for both sources, and parses the content into a PipelinesConfig.
func ParsePipelinesRef(
	driver driver.Interface,
	resource runtime.Object,
	pipelinesRef *commonv1.ConfigMapOrSecretSource,
	key string,
) (*Config, error) {
	parsed, err := common.ParseConfigMapOrSecretRefToConfig(
		driver, resource, pipelinesRef, key, "pipelinesRef",
		SecretRefWatchName, ConfigMapRefWatchName,
		Options,
	)
	if err != nil {
		return nil, err
	}
	return (*Config)(parsed), nil
}
