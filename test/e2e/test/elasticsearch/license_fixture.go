// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"encoding/json"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/chrono"
	"github.com/google/uuid"
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
	return json.Marshal(licenseSpec{
		UID:                e.UID,
		LicenseType:        e.Type,
		IssueDateInMillis:  e.IssueDateInMillis,
		ExpiryDateInMillis: e.ExpiryDateInMillis,
		MaxNodes:           nil, // assume v5 license w/ ERU
		MaxResourceUnits:   e.MaxResourceUnits,
		IssuedTo:           e.IssuedTo,
		Issuer:             e.Issuer,
		StartDateInMillis:  e.StartDateInMillis,
	})
}

func (e ESLicense) Version() int {
	return 5 // we only test the new enterprise capable license version
}

var _ license.Signable = &ESLicense{}

func GenerateTestLicense(signer *license.Signer) (client.License, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return client.License{}, err
	}
	licenseSpec := client.License{
		UID:                uuid.String(),
		Type:               "enterprise",
		IssueDateInMillis:  chrono.ToMillis(time.Now().Add(-3 * 24 * time.Hour)),
		ExpiryDateInMillis: chrono.ToMillis(time.Now().Add(30 * 24 * time.Hour)),
		MaxResourceUnits:   100,
		IssuedTo:           "ECK CI",
		Issuer:             "ECK e2e job",
		StartDateInMillis:  chrono.ToMillis(time.Now().Add(-3 * 24 * time.Hour)),
	}
	sign, err := signer.Sign(ESLicense{License: licenseSpec})
	if err != nil {
		return client.License{}, err
	}
	licenseSpec.Signature = string(sign)
	return licenseSpec, nil
}
