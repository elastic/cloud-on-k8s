// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pipelines

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/driver"
)

// RefWatchName returns the name of the watch registered on the secret referenced in `pipelinesRef`.
func RefWatchName(resource types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-pipelinesref", resource.Namespace, resource.Name)
}

// ParsePipelinesRef retrieves the content of a secret referenced in `pipelinesRef`, sets up dynamic watches for that secret,
// and parses the secret content into a PipelinesConfig.
func ParsePipelinesRef(
	driver driver.Interface,
	resource runtime.Object,
	pipelinesRef *commonv1.ConfigSource,
	secretKey string, // retrieve config data from that entry in the secret
) (*Config, error) {
	parsed, err := common.ParseConfigRefToConfig(driver, resource, pipelinesRef, secretKey, RefWatchName, Options)
	if err != nil {
		return nil, err
	}

	return (*Config)(parsed), nil
}
