// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package bucket

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"log"
	"regexp"
	"strings"
	"text/template"

	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/exec"
)

const (
	// ManagedByTag is the cloud resource tag/label key (underscore for GCP compatibility).
	ManagedByTag = "managed_by"
	// ManagedByValue is the expected value for the managed-by label/tag.
	ManagedByValue = "eck-deployer"
)

// Manager defines the interface for cloud storage bucket lifecycle operations.
type Manager interface {
	// Create creates a bucket, scoped credentials, and a Kubernetes Secret with those credentials.
	// The caller must ensure the current kubectl context points to the correct cluster.
	Create() error
	// Delete removes the cloud bucket, its contents, and associated cloud credentials (IAM user, service account, etc.).
	Delete() error
}

// Config holds common bucket configuration shared across providers.
type Config struct {
	// Name is the bucket name (already resolved from template).
	Name string
	// StorageClass is the cloud storage class (e.g. "standard", "STANDARD").
	StorageClass string
	// Labels are resource labels/tags for cost tracking and governance.
	Labels map[string]string
	// Region is the cloud region where the bucket should be created.
	Region string
	// SecretName is the name of the Kubernetes Secret to create.
	SecretName string
	// SecretNamespace is the namespace for the Kubernetes Secret.
	SecretNamespace string
}

// S3Config holds AWS S3-specific configuration for IAM user provisioning.
type S3Config struct {
	// IAMUserPath is the IAM path under which storage users are created.
	IAMUserPath string
	// ManagedPolicyARN is the ARN of a pre-existing managed policy to should be attached to IAM users.
	ManagedPolicyARN string
}

// isNotFound returns true if the command output contains any of the given not-found indicators.
// Use this to distinguish "resource doesn't exist" errors from authentication, network, or permission failures.
// NOTE: This shell-output-parsing utility would not be necessary if using a cloud provider's official API/SDK,
// which typically returns structured error responses for resource existence checks.
func isNotFound(cmdOutput string, indicators ...string) bool {
	for _, indicator := range indicators {
		if strings.Contains(cmdOutput, indicator) {
			return true
		}
	}
	return false
}

// safeNameRe matches names that are safe to interpolate into shell commands and YAML.
// Allows lowercase alphanumeric, digits, hyphens, underscores, and periods — the
// intersection of characters valid in S3/GCS/Azure bucket names and K8s resource names.
var safeNameRe = regexp.MustCompile(`^[a-z0-9._-]+$`)

// ValidateName checks that a resolved name is safe for use in shell commands and YAML.
func ValidateName(name, field string) error {
	if name == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if !safeNameRe.MatchString(name) {
		return fmt.Errorf("%s %q contains invalid characters: only lowercase alphanumeric, hyphens, underscores, and periods are allowed", field, name)
	}
	return nil
}

// ResolveName resolves template variables in a bucket name using the provided context.
func ResolveName(nameTemplate string, ctx map[string]any) (string, error) {
	tmpl, err := template.New("bucket-name").Parse(nameTemplate)
	if err != nil {
		return "", fmt.Errorf("while parsing bucket name template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("while resolving bucket name template: %w", err)
	}
	return buf.String(), nil
}

// createK8sSecret creates a Kubernetes Secret with the provided data map.
// The managed-by=eck-deployer label is included in the Secret at creation time.
// The caller (driver) must ensure the current kubectl context points to the correct cluster
// by calling GetCredentials() before any bucket operations.
func createK8sSecret(secretName, secretNamespace string, data map[string]string) error {
	log.Printf("Creating Kubernetes Secret %s/%s for bucket credentials...", secretNamespace, secretName)

	// Ensure namespace exists
	nsCmd := fmt.Sprintf(`kubectl create namespace %s --dry-run=client -o yaml | kubectl apply -f -`, secretNamespace)
	if err := exec.NewCommand(nsCmd).Run(); err != nil {
		return fmt.Errorf("while ensuring namespace %s: %w", secretNamespace, err)
	}

	// Build the Secret YAML with the label included at creation time
	var dataEntries strings.Builder
	for k, v := range data {
		dataEntries.WriteString(fmt.Sprintf("  %s: %s\n", k, base64.StdEncoding.EncodeToString([]byte(v))))
	}

	secretYAML := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
  labels:
    managed-by: eck-deployer
type: Opaque
data:
%s`, secretName, secretNamespace, dataEntries.String())

	cmd := fmt.Sprintf(`cat <<'EOF' | kubectl apply -f -
%s
EOF`, secretYAML)

	if err := exec.NewCommand(cmd).Run(); err != nil {
		return fmt.Errorf("while creating Kubernetes Secret %s/%s: %w", secretNamespace, secretName, err)
	}
	return nil
}
