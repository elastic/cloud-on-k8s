// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package bucket provides cloud storage utilities for E2E test cleanup.
package bucket

import (
	"fmt"
)

// Secret data keys used by Elasticsearch repository plugins.
// These are key names for looking up credentials in K8s Secret data, not actual credentials.
const (
	// GCS keys
	GCSCredentialsFileKey = "gcs.client.default.credentials_file" //nolint:gosec // key name, not a credential

	// S3 keys
	S3AccessKeyKey = "s3.client.default.access_key"
	S3SecretKeyKey = "s3.client.default.secret_key" //nolint:gosec // key name, not a credential

	// Azure keys
	AzureAccountKey  = "azure.client.default.account"
	AzureSASTokenKey = "azure.client.default.sas_token"
)

// Credentials holds cloud storage credentials extracted from a K8s Secret.
type Credentials struct {
	// Provider is the cloud provider: gcs, s3, or azure.
	Provider string
	// Bucket is the bucket name (GCS, S3) or container name (Azure).
	Bucket string
	// Region is the AWS region (S3 only).
	Region string
	// GCSCredentialsJSON is the service account JSON for GCS.
	GCSCredentialsJSON []byte
	// S3AccessKey is the AWS access key ID.
	S3AccessKey string
	// S3SecretKey is the AWS secret access key.
	S3SecretKey string
	// AzureAccount is the Azure storage account name.
	AzureAccount string
	// AzureSASToken is the Azure SAS token.
	AzureSASToken string
}

// CredentialsFromSecretData extracts bucket credentials from K8s Secret data.
// The provider, bucket, and region should come from Secret annotations.
func CredentialsFromSecretData(provider, bucket, region string, data map[string][]byte) (Credentials, error) {
	creds := Credentials{
		Provider: provider,
		Bucket:   bucket,
		Region:   region,
	}

	switch provider {
	case "gcs":
		credFile, ok := data[GCSCredentialsFileKey]
		if !ok {
			return creds, fmt.Errorf("missing %s in secret data", GCSCredentialsFileKey)
		}
		creds.GCSCredentialsJSON = credFile

	case "s3":
		accessKey, ok := data[S3AccessKeyKey]
		if !ok {
			return creds, fmt.Errorf("missing %s in secret data", S3AccessKeyKey)
		}
		secretKey, ok := data[S3SecretKeyKey]
		if !ok {
			return creds, fmt.Errorf("missing %s in secret data", S3SecretKeyKey)
		}
		creds.S3AccessKey = string(accessKey)
		creds.S3SecretKey = string(secretKey)

	case "azure":
		account, ok := data[AzureAccountKey]
		if !ok {
			return creds, fmt.Errorf("missing %s in secret data", AzureAccountKey)
		}
		sasToken, ok := data[AzureSASTokenKey]
		if !ok {
			return creds, fmt.Errorf("missing %s in secret data", AzureSASTokenKey)
		}
		creds.AzureAccount = string(account)
		creds.AzureSASToken = string(sasToken)

	default:
		return creds, fmt.Errorf("unsupported provider: %s", provider)
	}

	return creds, nil
}
