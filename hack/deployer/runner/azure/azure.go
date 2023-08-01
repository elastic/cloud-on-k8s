// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package azure

import (
	"encoding/json"
	"fmt"

	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/exec"
	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/runner/env"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/vault"
)

type Credentials struct {
	ClientID       string
	ClientSecret   string
	TenantID       string
	SubscriptionID string
}

const (
	AKSVaultPath     = "ci-azr-k8s-operator"
	azureClientImage = "mcr.microsoft.com/azure-cli"
)

func NewCredentials(c vault.Client) (Credentials, error) {
	creds, err := vault.GetMany(c, AKSVaultPath, "appId", "password", "tenant", "subscription")
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
	str, err := cmd.WithoutStreaming().StdoutOnly().Output()
	if err != nil {
		return false, err
	}
	type response struct {
		Exists bool `json:"exists"`
	}
	var r response
	if err := json.Unmarshal([]byte(str), &r); err != nil {
		return false, fmt.Errorf("failure to parse %s:  %w", str, err)
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
		-w /home \
		{{.ClientImage}} \
		az {{Join .Args " "}}`)
	return cmd.AsTemplate(params)
}
