// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
)

type SourceClusterLicense struct {
	License client.License `json:"license"`
}

type SourceEnterpriseLicense struct {
	Data SourceLicenseData `json:"license"`
}

type SourceLicenseData struct {
	Status             string                 `json:"status,omitempty"`
	UID                string                 `json:"uid"`
	Type               string                 `json:"type"`
	IssueDate          *time.Time             `json:"issue_date,omitempty"`
	IssueDateInMillis  int64                  `json:"issue_date_in_millis"`
	ExpiryDate         *time.Time             `json:"expiry_date,omitempty"`
	ExpiryDateInMillis int64                  `json:"expiry_date_in_millis"`
	MaxInstances       int                    `json:"max_instances"`
	IssuedTo           string                 `json:"issued_to"`
	Issuer             string                 `json:"issuer"`
	StartDateInMillis  int64                  `json:"start_date_in_millis"`
	Signature          string                 `json:"signature,omitempty"`
	ClusterLicenses    []SourceClusterLicense `json:"cluster_licenses"`
}
