// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package fixtures

const (
	LicenseUpdateResponseSample       = `{"acknowledged":true,"license_status":"valid"}`
	LicenseFailedUpdateResponseSample = `{"acknowledged":true,"license_status":"invalid"}`
	LicenseGetSample                  = `{
	  "license" : {
		"status" : "active",
		"uid" : "893361dc-9749-4997-93cb-802e3d7fa4xx",
		"type" : "platinum",
		"issue_date" : "2019-01-22T00:00:00.000Z",
		"issue_date_in_millis" : 1548115200000,
		"expiry_date" : "2019-06-22T23:59:59.999Z",
		"expiry_date_in_millis" : 1561247999999,
		"max_nodes" : 100,
		"issued_to" : "unit-tests",
		"issuer" : "issuer",
		"start_date_in_millis" : 1548115200000
	  }
	}`
	LicenseSample = `{
		"status" : "active",
		"uid" : "893361dc-9749-4997-93cb-802e3d7fa4xx",
		"type" : "platinum",
		"issue_date" : "2019-01-22T00:00:00.000Z",
		"issue_date_in_millis" : 1548115200000,
		"expiry_date" : "2019-06-22T23:59:59.999Z",
		"expiry_date_in_millis" : 1561247999999,
		"max_nodes" : 100,
		"issued_to" : "unit-tests",
		"issuer" : "issuer",
		"start_date_in_millis" : 1548115200000
    }`
)
