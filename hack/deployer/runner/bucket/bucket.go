// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package bucket

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"hash/fnv"
	"log"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/exec"
)

// fnv32 returns the FNV-1a 32-bit hash of s.
func fnv32(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

const (
	// managedByTag is the cloud resource tag/label key (underscore for GCP compatibility).
	managedByTag = "managed_by"
	// managedByValue is the expected value for the managed-by label/tag.
	managedByValue = "eck-deployer"
)

// Manager defines the interface for cloud storage bucket lifecycle operations.
type Manager interface {
	// Create creates a bucket, scoped credentials, and a Kubernetes Secret with those credentials.
	// The caller must ensure the current kubectl context points to the correct cluster.
	Create() error
	// Delete removes the cloud bucket, its contents, and associated cloud credentials (IAM user, service account, etc.).
	// It does not delete the Kubernetes Secret, which is expected to be cleaned up when the cluster is destroyed.
	Delete() error
}

// Config holds common bucket configuration shared across providers.
type Config struct {
	// Name is the bucket name (already resolved from template).
	Name string
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
	// ManagedPolicyARN is the ARN of a pre-existing managed policy that is attached to IAM users.
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
// Allows lowercase letters, digits, hyphens, underscores, and periods — the
// intersection of characters valid in S3/GCS/Azure bucket names and K8s resource names.
var safeNameRe = regexp.MustCompile(`^[a-z0-9._-]+$`)

// ValidateName checks that a resolved name contains only lowercase letters, digits,
// hyphens, underscores, and periods. Use this for bucket names, K8s resource names, GCP labels,
// cluster names, and plan IDs — all of which require lowercase.
// See also ValidateShellArg for a broader check that permits uppercase, colons, and slashes.
func ValidateName(name, field string) error {
	if name == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if !safeNameRe.MatchString(name) {
		return fmt.Errorf("%s %q contains invalid characters: only lowercase alphanumeric, hyphens, underscores, and periods are allowed", field, name)
	}
	return nil
}

// shellSafeRe is broader than safeNameRe: it additionally allows uppercase letters, colons,
// and slashes needed by GCS storage classes (STANDARD), IAM paths (/eck-deployer/), and
// ARNs (arn:aws:iam::123456789012:policy/Name).
var shellSafeRe = regexp.MustCompile(`^[a-zA-Z0-9_.:/-]+$`)

// ValidateShellArg checks that a value is safe for interpolation into shell commands.
// Use this for provider-specific fields (GCP projects, storage classes, IAM paths, ARNs,
// resource groups) that may contain uppercase letters, colons, or slashes but must not
// contain shell metacharacters.
// See also ValidateName for a stricter lowercase-only check used for bucket and K8s names.
func ValidateShellArg(value, field string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if !shellSafeRe.MatchString(value) {
		return fmt.Errorf("%s %q contains invalid characters: only alphanumeric, hyphens, underscores, periods, colons, and slashes are allowed", field, value)
	}
	return nil
}

// ResolveName resolves template variables in a bucket name using the provided context.
func ResolveName(nameTemplate string, ctx map[string]any) (string, error) {
	tmpl, err := template.New("bucket-name").Option("missingkey=error").Parse(nameTemplate)
	if err != nil {
		return "", fmt.Errorf("while parsing bucket name template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("while resolving bucket name template: %w", err)
	}
	return buf.String(), nil
}

// k8sSecretExists returns true if a Kubernetes Secret with the given name exists in the given namespace.
func k8sSecretExists(secretName, secretNamespace string) bool {
	cmd := fmt.Sprintf("kubectl get secret %s -n %s", secretName, secretNamespace)
	return exec.NewCommand(cmd).WithoutStreaming().Run() == nil
}

// k8sSecretAnnotation reads a single annotation value from a Kubernetes Secret.
// Returns the annotation value (may be empty) and any error from kubectl.
func k8sSecretAnnotation(secretName, secretNamespace, annotation string) (string, error) {
	cmd := fmt.Sprintf(
		`kubectl get secret %s -n %s -o jsonpath='{.metadata.annotations["%s"]}'`,
		secretName, secretNamespace, annotation,
	)
	output, err := exec.NewCommand(cmd).WithoutStreaming().Output()
	if err != nil {
		return "", fmt.Errorf("while reading annotation %s from Secret %s/%s: %w", annotation, secretNamespace, secretName, err)
	}
	return strings.TrimSpace(output), nil
}

// createK8sSecret creates a Kubernetes Secret with the provided data map.
// The managed-by=eck-deployer label is included in the Secret at creation time.
// The caller (driver) must ensure the current kubectl context points to the correct cluster
// by calling GetCredentials() before any bucket operations.
func createK8sSecret(secretName, secretNamespace string, data map[string]string, annotations map[string]string) error {
	log.Printf("Creating Kubernetes Secret %s/%s for bucket credentials...", secretNamespace, secretName)

	// Ensure namespace exists
	nsCmd := fmt.Sprintf(`kubectl create namespace %s --dry-run=client -o yaml | kubectl apply -f -`, secretNamespace)
	if err := exec.NewCommand(nsCmd).Run(); err != nil {
		return fmt.Errorf("while ensuring namespace %s: %w", secretNamespace, err)
	}

	// Build the Secret YAML with the label included at creation time.
	// Keys are sorted for deterministic output.
	var dataEntries strings.Builder
	for _, k := range slices.Sorted(maps.Keys(data)) {
		fmt.Fprintf(&dataEntries, "  %s: %s\n", k, base64.StdEncoding.EncodeToString([]byte(data[k])))
	}

	var annotationEntries strings.Builder
	for _, k := range slices.Sorted(maps.Keys(annotations)) {
		fmt.Fprintf(&annotationEntries, "    %s: %s\n", k, annotations[k])
	}

	annotationsBlock := ""
	if annotationEntries.Len() > 0 {
		annotationsBlock = fmt.Sprintf("  annotations:\n%s", annotationEntries.String())
	}

	secretYAML := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
%s  labels:
    managed-by: eck-deployer
type: Opaque
data:
%s`, secretName, secretNamespace, annotationsBlock, dataEntries.String())

	// Delete any existing Secret before creating, so that this function can be used
	// for both initial creation and credential rotation.
	// We use "kubectl create" instead of "kubectl apply" to avoid storing credentials
	// in the kubectl.kubernetes.io/last-applied-configuration annotation.
	deleteCmd := fmt.Sprintf("kubectl delete secret %s -n %s --ignore-not-found", secretName, secretNamespace)
	if err := exec.NewCommand(deleteCmd).WithoutStreaming().Run(); err != nil {
		return fmt.Errorf("while deleting existing Secret %s/%s: %w", secretNamespace, secretName, err)
	}

	tmpFile, err := os.CreateTemp("", "k8s-secret-*.yaml")
	if err != nil {
		return fmt.Errorf("while creating temp file for Secret YAML: %w", err)
	}
	defer os.Remove(filepath.Clean(tmpFile.Name()))

	if _, err := tmpFile.WriteString(secretYAML); err != nil {
		tmpFile.Close()
		return fmt.Errorf("while writing Secret YAML to temp file: %w", err)
	}
	tmpFile.Close()

	cmd := fmt.Sprintf("kubectl create -f %s", tmpFile.Name())
	if err := exec.NewCommand(cmd).WithoutStreaming().Run(); err != nil {
		return fmt.Errorf("while creating Kubernetes Secret %s/%s: %w", secretNamespace, secretName, err)
	}
	return nil
}
