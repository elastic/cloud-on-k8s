// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package bucket

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/exec"
)

// GCSManager manages Google Cloud Storage buckets.
type GCSManager struct {
	cfg          Config
	project      string
	storageClass string
}

var _ Manager = &GCSManager{}

// NewGCSManager creates a new GCS bucket manager. The storageClass parameter is
// GCS-specific (e.g. "STANDARD", "NEARLINE") and not part of the shared Config.
func NewGCSManager(cfg Config, project, storageClass string) *GCSManager {
	return &GCSManager{
		cfg:          cfg,
		project:      project,
		storageClass: storageClass,
	}
}

func (g *GCSManager) serviceAccountName() string {
	// GCP service account names must be 6-30 characters, lowercase, and match [a-z][a-z0-9-]*[a-z0-9].
	// The name includes a hash suffix to avoid collisions when long names are truncated,
	// matching the approach used by AzureManager.storageAccountName().
	hash := fmt.Sprintf("%08x", fnv32(g.cfg.Name))

	// Strip characters not allowed in GCP service account names.
	var cleaned []byte
	for _, c := range []byte(strings.ToLower(g.cfg.Name)) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			cleaned = append(cleaned, c)
		}
	}

	// Layout: "eck-bkt-" (8) + readable (up to 14) + hash (8) = 30 max
	const prefix = "eck-bkt-"
	if maxReadable := 30 - len(prefix) - len(hash); len(cleaned) > maxReadable {
		cleaned = cleaned[:maxReadable]
	}
	name := prefix + string(cleaned) + hash
	// Remove trailing hyphens (can happen if truncation lands on one)
	name = strings.TrimRight(name, "-")
	return name
}

func (g *GCSManager) serviceAccountEmail() string {
	return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", g.serviceAccountName(), g.project)
}

// Create creates a GCS bucket, a scoped service account, and a Kubernetes Secret with the credentials.
func (g *GCSManager) Create() error {
	if err := g.createBucket(); err != nil {
		return err
	}
	if err := g.blockPublicAccess(); err != nil {
		return err
	}
	if k8sSecretExists(g.cfg.SecretName, g.cfg.SecretNamespace) {
		log.Printf("Secret %s/%s already exists, skipping credential creation", g.cfg.SecretNamespace, g.cfg.SecretName)
		return nil
	}
	keyFile, err := g.createServiceAccountAndKey()
	if err != nil {
		return err
	}
	defer os.Remove(keyFile)

	keyData, err := os.ReadFile(keyFile)
	if err != nil {
		return fmt.Errorf("while reading service account key: %w", err)
	}

	var keyJSON struct {
		PrivateKeyID string `json:"private_key_id"`
	}
	if err := json.Unmarshal(keyData, &keyJSON); err != nil {
		return fmt.Errorf("while parsing service account key JSON: %w", err)
	}

	return createK8sSecret(g.cfg.SecretName, g.cfg.SecretNamespace, map[string]string{
		"gcs.client.default.credentials_file": string(keyData),
	}, map[string]string{
		"eck-deployer/service-account": g.serviceAccountEmail(),
		"eck-deployer/key-id":          keyJSON.PrivateKeyID,
	})
}

// Delete removes the GCS bucket, its contents, and the associated service account.
// Both deletions are attempted even if one fails to avoid leaking cloud resources.
// Each sub-function verifies ownership before deleting (display name for the service account, managed_by label for the bucket).
func (g *GCSManager) Delete() error {
	bucketErr := g.deleteBucket()
	saErr := g.deleteServiceAccount()
	return errors.Join(bucketErr, saErr)
}

func (g *GCSManager) createBucket() error {
	log.Printf("Creating GCS bucket %s in project %s...", g.cfg.Name, g.project)

	// Check if bucket already exists
	checkCmd := fmt.Sprintf("gcloud storage buckets describe gs://%s --project %s", g.cfg.Name, g.project)
	output, err := exec.NewCommand(checkCmd).WithoutStreaming().Output()
	if err == nil {
		log.Printf("Bucket %s already exists, skipping creation", g.cfg.Name)
		return nil
	}
	// "NOT_FOUND" is the GCP API error code, "not found" is the human-readable
	// message used by newer gcloud CLI versions (e.g. "gs://bucket not found: 404").
	if !isNotFound(output, "NOT_FOUND", "BucketNotFoundException", "not found") {
		return fmt.Errorf("while checking if bucket %s exists: %w", g.cfg.Name, err)
	}

	storageClassArg := ""
	if g.storageClass != "" {
		storageClassArg = fmt.Sprintf(" --default-storage-class=%s", g.storageClass)
	}

	createCmd := fmt.Sprintf(
		"gcloud storage buckets create gs://%s --project %s --location %s --uniform-bucket-level-access%s",
		g.cfg.Name, g.project, g.cfg.Region, storageClassArg,
	)
	if err := exec.NewCommand(createCmd).Run(); err != nil {
		return err
	}

	// Labels must be applied separately: gcloud storage buckets create does not support --labels.
	return g.labelBucket()
}

// labelBucket applies resource labels to the GCS bucket. This is a separate call because
// gcloud storage buckets create does not support the --labels flag.
func (g *GCSManager) labelBucket() error {
	if len(g.cfg.Labels) == 0 {
		return nil
	}
	labelParts := make([]string, 0, len(g.cfg.Labels))
	for k, v := range g.cfg.Labels {
		labelParts = append(labelParts, fmt.Sprintf("%s=%s", k, v))
	}

	log.Printf("Labeling GCS bucket %s...", g.cfg.Name)
	cmd := fmt.Sprintf(
		"gcloud storage buckets update gs://%s --project %s --update-labels=%s",
		g.cfg.Name, g.project, strings.Join(labelParts, ","),
	)
	return exec.NewCommand(cmd).Run()
}

// blockPublicAccess ensures public access prevention is enabled on the bucket.
// This may already be enforced by organization policy, but we set it explicitly
// to guard against org-level misconfiguration.
func (g *GCSManager) blockPublicAccess() error {
	log.Printf("Ensuring public access prevention is enabled on bucket %s...", g.cfg.Name)
	cmd := fmt.Sprintf(
		"gcloud storage buckets update gs://%s --public-access-prevention",
		g.cfg.Name,
	)
	return exec.NewCommand(cmd).Run()
}

func (g *GCSManager) createServiceAccountAndKey() (string, error) {
	saName := g.serviceAccountName()
	saEmail := g.serviceAccountEmail()

	log.Printf("Creating GCS service account %s...", saName)

	// Check if service account already exists
	checkCmd := fmt.Sprintf("gcloud iam service-accounts describe %s --project %s", saEmail, g.project)
	output, err := exec.NewCommand(checkCmd).WithoutStreaming().Output()
	if err == nil {
		log.Printf("Service account %s already exists, skipping creation", saName)
	} else if isNotFound(output, "NOT_FOUND", "PERMISSION_DENIED") {
		// GCP returns PERMISSION_DENIED instead of NOT_FOUND when the caller
		// lacks iam.serviceAccounts.get and the service account doesn't exist.
		// Create service account
		createCmd := fmt.Sprintf(
			`gcloud iam service-accounts create %s --display-name="Bucket SA for %s" --project %s`,
			saName, g.cfg.Name, g.project,
		)
		if err := exec.NewCommand(createCmd).Run(); err != nil {
			return "", fmt.Errorf("while creating service account: %w", err)
		}
	} else {
		return "", fmt.Errorf("while checking if service account %s exists: %w", saEmail, err)
	}

	// Grant objectAdmin role scoped to the bucket.
	// GCP IAM is eventually consistent: a newly created service account may not be
	// visible to other services for a few seconds, causing "does not exist" errors.
	log.Printf("Granting roles/storage.objectAdmin on bucket %s to %s...", g.cfg.Name, saEmail)
	bindCmd := fmt.Sprintf(
		`gcloud storage buckets add-iam-policy-binding gs://%s --member="serviceAccount:%s" --role="roles/storage.objectAdmin"`,
		g.cfg.Name, saEmail,
	)
	if err := retry(bindCmd, 5, 10*time.Second); err != nil {
		return "", fmt.Errorf("while granting bucket IAM binding: %w", err)
	}

	// Delete any existing user-managed keys to prevent orphaned keys
	// when a K8s Secret is deleted and recreated.
	if err := g.deleteExistingKeys(); err != nil {
		return "", err
	}

	// Generate JSON key
	keyFile, err := os.CreateTemp("", "gcs-sa-key-*.json")
	if err != nil {
		return "", fmt.Errorf("while creating temp key file: %w", err)
	}
	keyFile.Close()

	log.Printf("Creating service account key...")
	keyCmd := fmt.Sprintf(
		"gcloud iam service-accounts keys create %s --iam-account=%s --project %s",
		keyFile.Name(), saEmail, g.project,
	)
	if err := exec.NewCommand(keyCmd).Run(); err != nil {
		os.Remove(keyFile.Name())
		return "", fmt.Errorf("while creating service account key: %w", err)
	}

	return keyFile.Name(), nil
}

func (g *GCSManager) deleteExistingKeys() error {
	saEmail := g.serviceAccountEmail()
	listCmd := fmt.Sprintf(
		`gcloud iam service-accounts keys list --iam-account=%s --project %s --managed-by=user --format="value(name)"`,
		saEmail, g.project,
	)
	output, err := exec.NewCommand(listCmd).WithoutStreaming().Output()
	if err != nil {
		return fmt.Errorf("while listing keys for service account %s: %w", saEmail, err)
	}
	for keyID := range strings.FieldsSeq(strings.TrimSpace(output)) {
		log.Printf("Deleting existing key %s for service account %s...", keyID, saEmail)
		delCmd := fmt.Sprintf(
			"gcloud iam service-accounts keys delete %s --iam-account=%s --project %s --quiet",
			keyID, saEmail, g.project,
		)
		if err := exec.NewCommand(delCmd).WithoutStreaming().Run(); err != nil {
			return fmt.Errorf("while deleting key %s for service account %s: %w", keyID, saEmail, err)
		}
	}
	return nil
}

func (g *GCSManager) deleteServiceAccount() error {
	saEmail := g.serviceAccountEmail()
	log.Printf("Verifying GCS service account %s is managed by eck-deployer...", saEmail)

	// GCP service accounts don't support labels/tags. Verify via the display name
	// set at creation time ("Bucket SA for <bucket-name>").
	descCmd := fmt.Sprintf(
		`gcloud iam service-accounts describe %s --project %s --format="value(displayName)"`,
		saEmail, g.project,
	)
	output, err := exec.NewCommand(descCmd).WithoutStreaming().Output()
	if err != nil {
		if isNotFound(output, "NOT_FOUND", "PERMISSION_DENIED") {
			// GCP returns PERMISSION_DENIED instead of NOT_FOUND when the caller
			// lacks iam.serviceAccounts.get and the service account doesn't exist.
			log.Printf("Service account %s not found, skipping deletion", saEmail)
			return nil
		}
		return fmt.Errorf("while checking service account %s: %w", saEmail, err)
	}
	displayName := strings.TrimSpace(output)
	expectedPrefix := "Bucket SA for "
	if !strings.HasPrefix(displayName, expectedPrefix) {
		return fmt.Errorf(
			"refusing to delete GCS service account %s: display name %q does not start with %q. Delete it manually",
			saEmail, displayName, expectedPrefix,
		)
	}

	log.Printf("Deleting GCS service account %s...", saEmail)
	cmd := fmt.Sprintf("gcloud iam service-accounts delete %s --quiet --project %s", saEmail, g.project)
	if err := exec.NewCommand(cmd).Run(); err != nil {
		return fmt.Errorf("while deleting service account %s: %w", saEmail, err)
	}
	return nil
}

func (g *GCSManager) deleteBucket() error {
	log.Printf("Verifying GCS bucket %s is managed by eck-deployer...", g.cfg.Name)
	descCmd := fmt.Sprintf(
		`gcloud storage buckets describe gs://%s --project %s --format="value(labels.%s)"`,
		g.cfg.Name, g.project, ManagedByTag,
	)
	output, err := exec.NewCommand(descCmd).WithoutStreaming().Output()
	if err != nil {
		// "NOT_FOUND" is the GCP API error code, "not found" is the human-readable
		// message used by newer gcloud CLI versions (e.g. "gs://bucket not found: 404").
		if isNotFound(output, "NOT_FOUND", "BucketNotFoundException", "not found") {
			log.Printf("Bucket %s not found, skipping deletion", g.cfg.Name)
			return nil
		}
		return fmt.Errorf("while checking bucket %s: %w", g.cfg.Name, err)
	}
	value := strings.TrimSpace(output)
	if value != ManagedByValue {
		return fmt.Errorf(
			"refusing to delete GCS bucket %s: missing label %s=%s (found %q). Delete it manually",
			g.cfg.Name, ManagedByTag, ManagedByValue, value,
		)
	}

	log.Printf("Deleting GCS bucket %s and its contents...", g.cfg.Name)
	// --recursive on a bucket URI removes all objects and the bucket itself.
	cmd := fmt.Sprintf("gcloud storage rm --recursive gs://%s --project %s", g.cfg.Name, g.project)
	if err := exec.NewCommand(cmd).Run(); err != nil {
		return fmt.Errorf("while deleting bucket %s: %w", g.cfg.Name, err)
	}
	return nil
}

// retry runs a command up to maxAttempts times, sleeping between attempts.
// This is useful when a GCP resource was just created and may not have propagated yet.
func retry(cmd string, maxAttempts int, sleep time.Duration) error {
	var err error
	for i := range maxAttempts {
		if err = exec.NewCommand(cmd).Run(); err == nil {
			return nil
		}
		if i < maxAttempts-1 {
			log.Printf("Attempt %d/%d failed, retrying in %s...", i+1, maxAttempts, sleep)
			time.Sleep(sleep)
		}
	}
	return err
}
