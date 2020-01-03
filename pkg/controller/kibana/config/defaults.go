// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
)

// VersionDefaults generates any version specific settings that should exist by default.
func VersionDefaults(kb *kbv1.Kibana, v version.Version) *settings.CanonicalConfig {
	if v.IsSameOrAfter(version.From(7, 6, 0)) {
		// setting exists only as of 7.6.0
		return settings.MustCanonicalConfig(map[string]interface{}{XpackLicenseManagementUIEnabled: false})
	}

	return settings.NewCanonicalConfig()
}
