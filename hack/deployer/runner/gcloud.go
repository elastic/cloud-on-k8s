// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const (
	GCPDir                 = ".gcp"
	ServiceAccountFilename = "osServiceAccount.json"
)

// authToGCP authenticates the deployer to the Google Cloud Platform as a service account or as a user.
func authToGCP(
	vaultInfo *VaultInfo, vaultPath string, serviceAccountVaultFieldName string,
	asServiceAccount bool, configureDocker bool, gCloudProject interface{},
) error {
	if asServiceAccount {
		if vaultInfo == nil {
			return errors.New("vault info not present in the plan to authenticate to GCP")
		}

		log.Println("Authenticating as service account...")

		client, err := NewClient(*vaultInfo)
		if err != nil {
			return err
		}

		gcpDir := filepath.Join(os.Getenv("HOME"), GCPDir)
		keyFileName := filepath.Join(gcpDir, ServiceAccountFilename)
		if err = os.MkdirAll(gcpDir, os.ModePerm); err != nil {
			return err
		}

		if err := client.ReadIntoFile(keyFileName, vaultPath, serviceAccountVaultFieldName); err != nil {
			return err
		}

		// now that we're set on the cloud sdk directory, we can run any gcloud command that will rely on it
		if err := NewCommand(fmt.Sprintf("gcloud config set project %s", gCloudProject)).Run(); err != nil {
			return err
		}

		if err := NewCommand("gcloud auth activate-service-account --key-file=" + keyFileName).Run(); err != nil {
			return err
		}

		if configureDocker {
			return NewCommand("gcloud auth configure-docker").Run()
		}

		return nil
	}

	log.Println("Authenticating as user...")
	accounts, err := NewCommand(`gcloud auth list "--format=value(account)"`).StdoutOnly().WithoutStreaming().Output()
	if err != nil {
		return err
	}
	if len(accounts) > 0 {
		return nil
	}

	if err := NewCommand(fmt.Sprintf("gcloud config set project %s", gCloudProject)).Run(); err != nil {
		return err
	}

	return NewCommand("gcloud auth login").Run()
}
