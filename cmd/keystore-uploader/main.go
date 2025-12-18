// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package keystoreuploader provides a subcommand to upload a keystore file to a Kubernetes Secret.
// This is used by the keystore creation Job for Elasticsearch 9.3+ clusters.
//
// The Job runs in the operator namespace and creates a "staging" Secret there.
// The operator then copies this Secret to the target ES namespace during reconciliation.
// This design avoids needing per-namespace RBAC for the keystore job.
package keystoreuploader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	// KeystoreFileName is the name of the keystore file in the Secret data.
	KeystoreFileName = "elasticsearch.keystore"

	// defaultTimeout is the default timeout for the upload operation.
	defaultTimeout = 60 * time.Second
)

// Command returns the keystore-uploader cobra command.
func Command() *cobra.Command {
	var (
		keystorePath    string
		secretName      string
		namespace       string
		settingsHash    string
		sourceNamespace string
		sourceCluster   string
		timeout         time.Duration
	)

	cmd := &cobra.Command{
		Use:   "keystore-uploader",
		Short: "Upload a keystore file to a Kubernetes Secret",
		Long: `Upload a keystore file to a Kubernetes Secret.

This command is used internally by ECK to upload keystore files created by
the keystore init container to a staging Secret in the operator namespace.
The operator then copies this Secret to the target ES namespace.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Set up logging - use production mode (JSON output) for consistency with operator logs
			logf.SetLogger(zap.New())
			log := logf.Log.WithName("keystore-uploader")

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			ctx = ulog.AddToContext(ctx, log)

			return run(ctx, keystorePath, secretName, namespace, settingsHash, sourceNamespace, sourceCluster)
		},
	}

	cmd.Flags().StringVar(&keystorePath, "keystore-path", "/keystore/elasticsearch.keystore",
		"Path to the keystore file to upload")
	cmd.Flags().StringVar(&secretName, "secret-name", "",
		"Name of the staging Secret (in the operator namespace)")
	cmd.Flags().StringVar(&namespace, "namespace", "",
		"Namespace where to create the staging Secret (operator namespace)")
	cmd.Flags().StringVar(&settingsHash, "settings-hash", "",
		"Hash of the secure settings used to create this keystore")
	cmd.Flags().StringVar(&sourceNamespace, "source-namespace", "",
		"Namespace of the source Elasticsearch cluster (for debugging labels)")
	cmd.Flags().StringVar(&sourceCluster, "source-cluster", "",
		"Name of the source Elasticsearch cluster (for debugging labels)")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultTimeout,
		"Timeout for the upload operation")

	_ = cmd.MarkFlagRequired("secret-name")
	_ = cmd.MarkFlagRequired("namespace")
	_ = cmd.MarkFlagRequired("settings-hash")

	return cmd
}

// run executes the keystore upload logic.
func run(ctx context.Context, keystorePath, secretName, namespace, settingsHash, sourceNamespace, sourceCluster string) error {
	log := ulog.FromContext(ctx)
	log.Info("Reading keystore file", "path", keystorePath)

	// Read the keystore file
	keystoreData, err := os.ReadFile(keystorePath)
	if err != nil {
		return fmt.Errorf("failed to read keystore file: %w", err)
	}

	if len(keystoreData) == 0 {
		return fmt.Errorf("keystore file is empty")
	}

	// Compute SHA-256 digest of the keystore file
	digest := computeDigest(keystoreData)
	log.Info("Computed keystore digest", "digest", digest)

	// Create Kubernetes client
	k8sClient, err := createClient()
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Create or update the staging Secret (no owner reference - operator will copy it)
	if err := reconcileStagingSecret(ctx, k8sClient, secretName, namespace, keystoreData, settingsHash, digest, sourceNamespace, sourceCluster); err != nil {
		return fmt.Errorf("failed to reconcile staging secret: %w", err)
	}

	log.Info("Successfully uploaded keystore to staging Secret", "namespace", namespace, "secret", secretName)
	return nil
}

// computeDigest computes the SHA-256 digest of the given data and returns it as a hex string.
func computeDigest(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// createClient creates a new Kubernetes client using in-cluster config.
func createClient() (k8s.Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	// Register the Elasticsearch scheme (needed for labels/annotations constants)
	if err := esv1.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("failed to add Elasticsearch scheme: %w", err)
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return k8sClient, nil
}

// reconcileStagingSecret creates or updates the staging Secret in the operator namespace.
// This Secret has no owner reference since it's in a different namespace than the ES resource.
// The operator will copy this Secret to the target namespace and set the owner reference there.
func reconcileStagingSecret(
	ctx context.Context,
	k8sClient k8s.Client,
	secretName, namespace string,
	keystoreData []byte,
	settingsHash, digest, sourceNamespace, sourceCluster string,
) error {
	// Build labels - include source cluster info for debugging since secret names are hashed
	labels := map[string]string{
		"app.kubernetes.io/name":       "eck-keystore-job",
		"app.kubernetes.io/managed-by": "eck-operator",
	}
	if sourceNamespace != "" {
		labels[label.SourceNamespaceLabelName] = sourceNamespace
	}
	if sourceCluster != "" {
		labels[label.SourceClusterLabelName] = sourceCluster
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels:    labels,
			Annotations: map[string]string{
				esv1.KeystoreHashAnnotation:   settingsHash,
				esv1.KeystoreDigestAnnotation: digest,
			},
		},
		Data: map[string][]byte{
			KeystoreFileName: keystoreData,
		},
	}

	// Get existing secret if it exists
	var existing corev1.Secret
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&expected), &existing)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to get existing secret: %w", err)
		}
		// Secret doesn't exist, create it
		if err := k8sClient.Create(ctx, &expected); err != nil {
			return fmt.Errorf("failed to create secret: %w", err)
		}
		return nil
	}

	// Secret exists, update it
	existing.Data = expected.Data
	existing.Labels = labels
	if existing.Annotations == nil {
		existing.Annotations = make(map[string]string)
	}
	existing.Annotations[esv1.KeystoreHashAnnotation] = settingsHash
	existing.Annotations[esv1.KeystoreDigestAnnotation] = digest

	if err := k8sClient.Update(ctx, &existing); err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}
	return nil
}
