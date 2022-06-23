// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/chrono"
)

type ESLicense struct {
	client.License
}

type licenseSpec struct {
	UID                string `json:"uid"`
	LicenseType        string `json:"type"`
	IssueDateInMillis  int64  `json:"issue_date_in_millis,omitempty"`
	ExpiryDateInMillis int64  `json:"expiry_date_in_millis,omitempty"`
	MaxNodes           *int   `json:"max_nodes"`
	MaxResourceUnits   int    `json:"max_resource_units,omitempty"`
	IssuedTo           string `json:"issued_to"`
	Issuer             string `json:"issuer"`
	StartDateInMillis  int64  `json:"start_date_in_millis,omitempty"`
}

func (e ESLicense) SignableContentBytes() ([]byte, error) {
	spec := licenseSpec{
		UID:                e.UID,
		LicenseType:        e.Type,
		IssueDateInMillis:  e.IssueDateInMillis,
		ExpiryDateInMillis: e.ExpiryDateInMillis,
		MaxNodes:           &e.MaxNodes,
		MaxResourceUnits:   e.MaxResourceUnits,
		IssuedTo:           e.IssuedTo,
		Issuer:             e.Issuer,
		StartDateInMillis:  e.StartDateInMillis,
	}

	// v3 and v5 are handling maxNodes differently: v3 requires max_nodes v5 does not tolerate any other value than null
	if e.MaxNodes == 0 {
		spec.MaxNodes = nil
	}

	return json.Marshal(spec)
}

func (e ESLicense) Version() int {
	if e.Type == string(client.ElasticsearchLicenseTypeTrial) {
		return 3 // we either test trials
	}
	return 5 // or the new enterprise capable license version
}

var _ license.Signable = &ESLicense{}

func GenerateTestLicense(signer *license.Signer, typ client.ElasticsearchLicenseType) (client.License, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return client.License{}, err
	}
	licenseSpec := client.License{
		UID:                uuid.String(),
		Type:               string(typ),
		IssueDateInMillis:  chrono.ToMillis(time.Now().Add(-3 * 24 * time.Hour)),
		ExpiryDateInMillis: chrono.ToMillis(time.Now().Add(30 * 24 * time.Hour)),
		MaxResourceUnits:   100,
		IssuedTo:           "ECK CI",
		Issuer:             "ECK e2e job",
		StartDateInMillis:  chrono.ToMillis(time.Now().Add(-3 * 24 * time.Hour)),
	}
	if typ == client.ElasticsearchLicenseTypeTrial {
		licenseSpec.MaxResourceUnits = 0
		licenseSpec.MaxNodes = 100
	}

	sign, err := signer.Sign(ESLicense{License: licenseSpec})
	if err != nil {
		return client.License{}, err
	}
	licenseSpec.Signature = string(sign)
	return licenseSpec, nil
}
