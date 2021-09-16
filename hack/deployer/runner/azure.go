// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import "encoding/json"

// TODO this should probably be a package

type azureCredentials struct {
	ClientID       string
	ClientSecret   string
	TenantID       string
	SubscriptionID string
}

func newAzureCredentials(client *VaultClient) (azureCredentials, error) {
	creds, err := client.GetMany(AksVaultPath, "appId", "password", "tenant", "subscription")
	if err != nil {
		return azureCredentials{}, err
	}
	return azureCredentials{
		ClientID:       creds[0],
		ClientSecret:   creds[1],
		TenantID:       creds[2],
		SubscriptionID: creds[3],
	}, nil
}

func azureLogin(creds azureCredentials) error {
	cmd := "az login --service-principal -u {{.AppId}} -p {{.TenantSecret}} --tenant {{.TenantId}}"
	return NewCommand(cmd).
		AsTemplate(map[string]interface{}{
			"AppId":        creds.ClientID,
			"TenantSecret": creds.ClientSecret,
			"TenantId":     creds.TenantID,
		}).
		WithoutStreaming().
		Run()
}

func azureExistsCmd(cmd *Command) (bool, error) {
	str, err := cmd.WithoutStreaming().Output()
	if err != nil {
		return false, err
	}
	type response struct {
		Exists bool `json:"exists"`
	}
	var r response
	if err := json.Unmarshal([]byte(str), &r); err != nil {
		return false, err
	}
	return r.Exists, nil
}
