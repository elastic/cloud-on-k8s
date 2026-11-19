// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package version

import "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"

// PluginsMinVersion is the minimum Elasticsearch version supported for spec.plugins.
// Earlier versions fail to restart when elasticsearch-plugins.yml lists an official plugin
// without a location and the plugins cache file exists (elastic/elasticsearch#114818).
var PluginsMinVersion = version.MinFor(8, 17, 0)
