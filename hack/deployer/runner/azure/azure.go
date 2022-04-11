// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package azure

import (
	"encoding/json"

	"github.com/elastic/cloud-on-k8s/hack/deployer/exec"
	"github.com/elastic/cloud-on-k8s/hack/deployer/runner/env"
	"github.com/elastic/cloud-on-k8s/hack/deployer/vault"
)

type Credentials struct {
	ClientID       string
	ClientSecret   string
	TenantID       string
	SubscriptionID string
}

const (
	AKSVaultPath     = "secret/devops-ci/cloud-on-k8s/ci-azr-k8s-operator"
	azureClientImage = "mcr.microsoft.com/azure-cli"
)

func NewCredentials(client *vault.Client) (Credentials, error) {
	creds, err := client.GetMany(AKSVaultPath, "appId", "password", "tenant", "subscription")
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{
		ClientID:       creds[0],
		ClientSecret:   creds[1],
		TenantID:       creds[2],
		SubscriptionID: creds[3],
	}, nil
}

func Login(creds Credentials) error {
	return Cmd("login", "--service-principal", "-u", creds.ClientID, "-p", creds.ClientSecret, "--tenant", creds.TenantID).
		WithoutStreaming().
		Run()
}

func ExistsCmd(cmd *exec.Command) (bool, error) {
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

func Cmd(args ...string) *exec.Command {
	params := map[string]interface{}{
		"SharedVolume": env.SharedVolumeName(),
		"ClientImage":  azureClientImage,
		"Args":         args,
	}
	cmd := exec.NewCommand(`docker run --rm \
		-v {{.SharedVolume}}:/home \
		-e HOME=/home \
		{{.ClientImage}} \
		az {{Join .Args " "}}`)
	return cmd.AsTemplate(params)
}
