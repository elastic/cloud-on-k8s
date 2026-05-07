// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package bucket

import (
	"fmt"
	"log"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/vault"
)

// Vault paths for pre-provisioned stateless bucket credentials.
// Uses flat naming convention: stateless-bucket-{provider}
const (
	StatelessGCSVaultPath   = "stateless-bucket-gcs"    // GCS bucket for GKE
	StatelessS3VaultPath    = "stateless-bucket-s3"     // S3 bucket for EKS
	StatelessS3ARMVaultPath = "stateless-bucket-s3-arm" // S3 bucket for EKS ARM (different region)
	StatelessAzureVaultPath = "stateless-bucket-azure"  // Azure Blob for AKS

	// StatelessSecretName is the name of the K8s Secret containing bucket credentials.
	StatelessSecretName = "elasticsearch-object-store"
	// StatelessSecretNamespace is the namespace where the bucket credentials Secret is created.
	StatelessSecretNamespace = "default"
)

// StatelessVaultPaths maps provider IDs to their Vault paths.
var StatelessVaultPaths = map[string]string{
	"gke":     StatelessGCSVaultPath,
	"ocp":     StatelessGCSVaultPath, // OCP runs on GCP, uses GCS
	"eks":     StatelessS3VaultPath,
	"eks-arm": StatelessS3ARMVaultPath,
	"aks":     StatelessAzureVaultPath,
	"kind":    StatelessGCSVaultPath, // Kind uses GCS bucket for stateless tests
	"k3d":     StatelessGCSVaultPath, // K3D uses GCS bucket for stateless tests
}

// VaultProvider identifies the cloud storage provider for Vault-based bucket credentials.
type VaultProvider string

const (
	VaultProviderGCS   VaultProvider = "gcs"
	VaultProviderS3    VaultProvider = "s3"
	VaultProviderAzure VaultProvider = "azure"
)

// VaultConfig holds configuration for reading bucket credentials from Vault.
type VaultConfig struct {
	// VaultPath is the path (relative to VAULT_ROOT_PATH) containing the bucket credentials.
	VaultPath string
	// Provider identifies the cloud storage provider (gcs, s3, azure).
	Provider VaultProvider
	// SecretName is the name of the Kubernetes Secret to create.
	SecretName string
	// SecretNamespace is the namespace for the Kubernetes Secret.
	SecretNamespace string
}

// VaultManager reads pre-provisioned bucket credentials from Vault and creates a Kubernetes Secret.
// Unlike other bucket managers, it does not create or delete cloud resources.
// Credentials are read eagerly at construction time so that the Vault token does not need
// to remain valid until Create() is called (important for OCP where cluster creation can
// take 30-45 minutes and Vault tokens may expire).
type VaultManager struct {
	cfg         VaultConfig
	secretData  map[string]string
	annotations map[string]string
}

// NewVaultManager creates a new VaultManager by reading bucket credentials from Vault.
// The read happens at construction time to avoid Vault token expiry during long-running
// operations (e.g., OCP cluster creation).
func NewVaultManager(cfg VaultConfig, vaultClient vault.Client) (*VaultManager, error) {
	log.Printf("Reading bucket credentials from Vault path: %s (provider: %s)", cfg.VaultPath, cfg.Provider)

	m := &VaultManager{cfg: cfg}
	var err error

	switch cfg.Provider {
	case VaultProviderGCS:
		m.secretData, m.annotations, err = readGCSCredentials(vaultClient, cfg.VaultPath)
	case VaultProviderS3:
		m.secretData, m.annotations, err = readS3Credentials(vaultClient, cfg.VaultPath)
	case VaultProviderAzure:
		m.secretData, m.annotations, err = readAzureCredentials(vaultClient, cfg.VaultPath)
	default:
		return nil, fmt.Errorf("unsupported vault provider: %s", cfg.Provider)
	}

	if err != nil {
		return nil, fmt.Errorf("while reading credentials from Vault: %w", err)
	}
	return m, nil
}

// Create creates a Kubernetes Secret from the credentials read at construction time.
func (m *VaultManager) Create() error {
	return CreateK8sSecret(m.cfg.SecretName, m.cfg.SecretNamespace, m.secretData, m.annotations)
}

// Delete is a no-op for VaultManager since we don't own the bucket.
// The bucket is pre-provisioned and should not be deleted by the deployer.
func (m *VaultManager) Delete() error {
	log.Printf("Skipping bucket deletion for Vault-managed bucket (bucket is pre-provisioned)")
	return nil
}

// readGCSCredentials reads GCS bucket credentials from Vault.
// Expected Vault keys: bucket, project, credentials_file
// The returned annotations provide bucket configuration for the E2E test framework,
// consistent with GCSManager.Create().
func readGCSCredentials(vaultClient vault.Client, vaultPath string) (map[string]string, map[string]string, error) {
	values, err := vault.GetMany(vaultClient, vaultPath, "bucket", "project", "credentials_file")
	if err != nil {
		return nil, nil, err
	}

	bucket, project, credentialsFile := values[0], values[1], values[2]

	secretData := map[string]string{
		"gcs.client.default.credentials_file": credentialsFile,
	}

	annotations := map[string]string{
		AnnotationProvider: ProviderGCS,
		AnnotationBucket:   bucket,
		AnnotationProject:  project,
		AnnotationSource:   "vault",
	}

	log.Printf("Read GCS credentials from Vault: bucket=%s, project=%s", bucket, project)
	return secretData, annotations, nil
}

// readS3Credentials reads S3 bucket credentials from Vault.
// Expected Vault keys: bucket, region, access_key, secret_key
// See readGCSCredentials for annotation documentation.
func readS3Credentials(vaultClient vault.Client, vaultPath string) (map[string]string, map[string]string, error) {
	values, err := vault.GetMany(vaultClient, vaultPath, "bucket", "region", "access_key", "secret_key")
	if err != nil {
		return nil, nil, err
	}

	bucket, region, accessKey, secretKey := values[0], values[1], values[2], values[3]

	secretData := map[string]string{
		"s3.client.default.access_key": accessKey,
		"s3.client.default.secret_key": secretKey,
	}

	annotations := map[string]string{
		AnnotationProvider: ProviderS3,
		AnnotationBucket:   bucket,
		AnnotationRegion:   region,
		AnnotationSource:   "vault",
	}

	log.Printf("Read S3 credentials from Vault: bucket=%s, region=%s", bucket, region)
	return secretData, annotations, nil
}

// readAzureCredentials reads Azure Blob Storage credentials from Vault.
// Expected Vault keys: storage_account, container, sas_token
// See readGCSCredentials for annotation documentation.
func readAzureCredentials(vaultClient vault.Client, vaultPath string) (map[string]string, map[string]string, error) {
	values, err := vault.GetMany(vaultClient, vaultPath, "storage_account", "container", "sas_token")
	if err != nil {
		return nil, nil, err
	}

	storageAccount, container, sasToken := values[0], values[1], values[2]

	secretData := map[string]string{
		"azure.client.default.account":   storageAccount,
		"azure.client.default.sas_token": sasToken,
	}

	annotations := map[string]string{
		AnnotationProvider:       ProviderAzure,
		AnnotationStorageAccount: storageAccount,
		AnnotationContainer:      container,
		AnnotationSource:         "vault",
	}

	log.Printf("Read Azure credentials from Vault: storage_account=%s, container=%s", storageAccount, container)
	return secretData, annotations, nil
}
