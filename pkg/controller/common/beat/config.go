// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"path"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	CAMountPath = "/mnt/elastic-internal/es-certs/"
	CAFileName  = "ca.crt"

	// ConfigChecksumLabel is a label used to store beats config checksum.
	ConfigChecksumLabel = "beat.k8s.elastic.co/config-checksum"

	// VersionLabelName is a label used to track the version of a Beat Pod.
	VersionLabelName = "beat.k8s.elastic.co/version"
)

// SetOutput will set output section in Beat config according to association configuration.
func SetOutput(cfg *settings.CanonicalConfig, client k8s.Client, associated commonv1.Associated) error {
	if associated.AssociationConf().IsConfigured() {
		username, password, err := association.ElasticsearchAuthSettings(client, associated)
		if err != nil {
			return err
		}

		return cfg.MergeWith(settings.MustCanonicalConfig(
			map[string]interface{}{
				"output.elasticsearch": map[string]interface{}{
					"hosts":                       []string{associated.AssociationConf().GetURL()},
					"username":                    username,
					"password":                    password,
					"ssl.certificate_authorities": path.Join(CAMountPath, CAFileName),
				},
			}))
	}

	return nil
}
