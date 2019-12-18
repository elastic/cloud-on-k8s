// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version7

import (
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/config"
)

// SettingsFactory returns Kibana settings for a 7.x Kibana.
func SettingsFactory(kb kbv1.Kibana, v version.Version) map[string]interface{} {
	settings := map[string]interface{}{
		config.ElasticsearchHosts: kb.AssociationConf().GetURL(),
	}
	if v.IsSameOrAfter(version.MustParse("7.6.0")) {
		// setting exists only as of 7.6.0
		settings[config.XpackLicenseManagementUIEnabled] = false
	}
	return settings
}
