// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package bucket

import (
	"fmt"
	"log"
	"strings"

	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/exec"
)

// AzureManager manages Azure Blob Storage containers.
type AzureManager struct {
	cfg           Config
	resourceGroup string
}

var _ Manager = &AzureManager{}

// NewAzureManager creates a new Azure Blob Storage manager.
func NewAzureManager(cfg Config, resourceGroup string) *AzureManager {
	return &AzureManager{
		cfg:           cfg,
		resourceGroup: resourceGroup,
	}
}

// storageAccountName returns a valid Azure storage account name.
// Storage account names must be 3-24 characters, lowercase alphanumeric only.
// The name is prefixed with "eckbkt" (alphanumeric form of "eck-bkt") to identify it as managed by the deployer.
func (a *AzureManager) storageAccountName() string {
	// Remove non-alphanumeric characters and lowercase
	raw := strings.ToLower(a.cfg.Name)
	var cleaned []byte
	for _, c := range []byte(raw) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			cleaned = append(cleaned, c)
		}
	}
	name := "eckbkt" + string(cleaned)
	if len(name) > 24 {
		name = name[:24]
	}
	return name
}

// containerName returns the name for the blob container within the storage account.
func (a *AzureManager) containerName() string {
	return "data"
}

// Create creates an Azure Storage account and blob container, then stores the account name
// and a SAS token in a Kubernetes Secret using the Elasticsearch Azure repository plugin key names.
func (a *AzureManager) Create() error {
	if err := a.createStorageAccount(); err != nil {
		return err
	}
	if err := a.createContainer(); err != nil {
		return err
	}
	sasToken, err := a.generateSASToken()
	if err != nil {
		return err
	}

	return createK8sSecret(a.cfg.SecretName, a.cfg.SecretNamespace, map[string]string{
		"azure.client.default.account":   a.storageAccountName(),
		"azure.client.default.sas_token": sasToken,
	})
}

// Delete removes the Azure Storage account and associated container.
// The sub-function verifies the managed_by tag before deleting.
func (a *AzureManager) Delete() error {
	return a.deleteStorageAccount()
}

func (a *AzureManager) createStorageAccount() error {
	accountName := a.storageAccountName()
	log.Printf("Creating Azure Storage account %s in resource group %s...", accountName, a.resourceGroup)

	// Check if storage account already exists
	checkCmd := fmt.Sprintf(
		"az storage account show --name %s --resource-group %s",
		accountName, a.resourceGroup,
	)
	output, err := exec.NewCommand(checkCmd).WithoutStreaming().Output()
	if err == nil {
		log.Printf("Storage account %s already exists, skipping creation", accountName)
		return nil
	}
	if !isNotFound(output, "ResourceNotFound", "was not found") {
		return fmt.Errorf("while checking if storage account %s exists: %w", accountName, err)
	}

	// Build tags string
	var tagParts []string
	for k, v := range a.cfg.Labels {
		tagParts = append(tagParts, fmt.Sprintf("%s=%s", k, v))
	}
	tagsArg := ""
	if len(tagParts) > 0 {
		tagsArg = fmt.Sprintf(" --tags %s", strings.Join(tagParts, " "))
	}

	cmd := fmt.Sprintf(
		"az storage account create --name %s --resource-group %s --location %s --sku Standard_LRS --allow-blob-public-access false --min-tls-version TLS1_2%s",
		accountName, a.resourceGroup, a.cfg.Region, tagsArg,
	)
	return exec.NewCommand(cmd).Run()
}

func (a *AzureManager) createContainer() error {
	containerName := a.containerName()
	accountName := a.storageAccountName()
	log.Printf("Creating Azure Blob container %s in account %s...", containerName, accountName)

	cmd := fmt.Sprintf(
		"az storage container create --name %s --account-name %s --auth-mode login",
		containerName, accountName,
	)
	return exec.NewCommand(cmd).Run()
}

// getAccountKey retrieves the primary access key for the storage account.
func (a *AzureManager) getAccountKey() (string, error) {
	accountName := a.storageAccountName()
	cmd := fmt.Sprintf(
		`az storage account keys list --account-name %s --resource-group %s --query "[0].value" --output tsv`,
		accountName, a.resourceGroup,
	)
	output, err := exec.NewCommand(cmd).WithoutStreaming().Output()
	if err != nil {
		return "", fmt.Errorf("while retrieving account key for %s: %w", accountName, err)
	}
	key := strings.TrimSpace(output)
	if key == "" {
		return "", fmt.Errorf("account key is empty for storage account %s", accountName)
	}
	return key, nil
}

// generateSASToken generates a Shared Access Signature (SAS) token for the storage account.
// The token is scoped to the Blob service with read, write, delete, list, add, create, and update
// permissions on all resource types (service, container, object), valid for 1 year.
func (a *AzureManager) generateSASToken() (string, error) {
	accountName := a.storageAccountName()
	log.Printf("Generating SAS token for storage account %s...", accountName)

	// Retrieve the account key explicitly to avoid credential lookup warnings in the output.
	accountKey, err := a.getAccountKey()
	if err != nil {
		return "", err
	}

	// Expiry set to 1 year from now. The token can be regenerated by re-running create.
	expiry := fmt.Sprintf(`$(date -u -d "+1 year" '+%%Y-%%m-%%dT%%H:%%MZ' 2>/dev/null || date -u -v+1y '+%%Y-%%m-%%dT%%H:%%MZ')`)

	cmd := fmt.Sprintf(
		`az storage account generate-sas --account-name %s --account-key %s --services b --resource-types sco --permissions rwdlacup --expiry %s --https-only --output tsv`,
		accountName, accountKey, expiry,
	)
	output, err := exec.NewCommand(cmd).WithoutStreaming().Output()
	if err != nil {
		return "", fmt.Errorf("while generating SAS token: %w", err)
	}
	token := strings.TrimSpace(output)
	if token == "" {
		return "", fmt.Errorf("SAS token is empty for storage account %s", accountName)
	}
	return token, nil
}

func (a *AzureManager) deleteStorageAccount() error {
	accountName := a.storageAccountName()
	log.Printf("Verifying Azure Storage account %s is managed by eck-deployer...", accountName)

	// Check the managed_by tag on the storage account
	tagCmd := fmt.Sprintf(
		`az storage account show --name %s --resource-group %s --query "tags.%s" --output tsv`,
		accountName, a.resourceGroup, ManagedByTag,
	)
	output, err := exec.NewCommand(tagCmd).WithoutStreaming().Output()
	if err != nil {
		if isNotFound(output, "ResourceNotFound", "was not found") {
			log.Printf("Storage account %s not found, skipping deletion", accountName)
			return nil
		}
		return fmt.Errorf("while checking storage account %s: %w", accountName, err)
	}
	value := strings.TrimSpace(output)
	if value != ManagedByValue {
		return fmt.Errorf(
			"refusing to delete Azure Storage account %s: missing tag %s=%s (found %q). Delete it manually",
			accountName, ManagedByTag, ManagedByValue, value,
		)
	}

	log.Printf("Deleting Azure Storage account %s...", accountName)
	cmd := fmt.Sprintf(
		"az storage account delete --name %s --resource-group %s --yes",
		accountName, a.resourceGroup,
	)
	if err := exec.NewCommand(cmd).Run(); err != nil {
		return fmt.Errorf("while deleting storage account %s: %w", accountName, err)
	}
	return nil
}
