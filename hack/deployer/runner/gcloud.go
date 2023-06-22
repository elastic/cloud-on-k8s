// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/exec"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/vault"
)

const (
	GCPDir                 = ".gcp"
	ServiceAccountFilename = "osServiceAccount.json"
)

// authToGCP authenticates the deployer to the Google Cloud Platform as a service account or as a user.
func authToGCP(
	client vault.Client, vaultPath string, serviceAccountVaultFieldName string,
	asServiceAccount bool, configureDocker bool, gCloudProject interface{},
) error {
	//nolint:nestif
	if asServiceAccount {
		log.Println("Authenticating as service account...")

		gcpDir := filepath.Join(os.Getenv("HOME"), GCPDir)
		keyFileName := filepath.Join(gcpDir, ServiceAccountFilename)
		if err := os.MkdirAll(gcpDir, os.ModePerm); err != nil {
			return err
		}

		clientProvider := func() (vault.Client, error) { return client, nil }
		_, err := vault.ReadFile(clientProvider, vault.SecretFile{
			Path:          vaultPath,
			Name:          keyFileName,
			FieldResolver: func() string { return serviceAccountVaultFieldName },
		})
		if err != nil {
			return err
		}

		// now that we're set on the cloud sdk directory, we can run any gcloud command that will rely on it
		if err := exec.NewCommand(fmt.Sprintf("gcloud config set project %s", gCloudProject)).Run(); err != nil {
			return err
		}

		if err := exec.NewCommand("gcloud auth activate-service-account --key-file=" + keyFileName).Run(); err != nil {
			return err
		}

		if configureDocker {
			return exec.NewCommand("gcloud auth configure-docker").Run()
		}

		return nil
	}

	log.Println("Authenticating as user...")
	accounts, err := exec.NewCommand(`gcloud auth list "--format=value(account)"`).StdoutOnly().WithoutStreaming().Output()
	if err != nil {
		return err
	}
	if len(accounts) > 0 {
		return nil
	}

	if err := exec.NewCommand(fmt.Sprintf("gcloud config set project %s", gCloudProject)).Run(); err != nil {
		return err
	}

	return exec.NewCommand("gcloud auth login").Run()
}
