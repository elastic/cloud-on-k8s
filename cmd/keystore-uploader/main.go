// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package keystoreuploader provides a subcommand to upload a keystore file to a Kubernetes Secret.
// This is used by the keystore creation Job for Elasticsearch 9.3+ clusters.
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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
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
		keystorePath string
		secretName   string
		namespace    string
		settingsHash string
		ownerName    string
		timeout      time.Duration
	)

	cmd := &cobra.Command{
		Use:   "keystore-uploader",
		Short: "Upload a keystore file to a Kubernetes Secret",
		Long: `Upload a keystore file to a Kubernetes Secret.

This command is used internally by ECK to upload keystore files created by
the keystore init container to a Kubernetes Secret. The Secret is then mounted
into Elasticsearch pods for the reloadable keystore feature (9.3+).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Set up logging - use production mode (JSON output) for consistency with operator logs
			logf.SetLogger(zap.New())
			log := logf.Log.WithName("keystore-uploader")

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			ctx = ulog.AddToContext(ctx, log)

			return run(ctx, keystorePath, secretName, namespace, settingsHash, ownerName)
		},
	}

	cmd.Flags().StringVar(&keystorePath, "keystore-path", "/keystore/elasticsearch.keystore",
		"Path to the keystore file to upload")
	cmd.Flags().StringVar(&secretName, "secret-name", "",
		"Name of the target Secret")
	cmd.Flags().StringVar(&namespace, "namespace", "",
		"Namespace of the target Secret")
	cmd.Flags().StringVar(&settingsHash, "settings-hash", "",
		"Hash of the secure settings used to create this keystore")
	cmd.Flags().StringVar(&ownerName, "owner-name", "",
		"Name of the Elasticsearch resource that owns this keystore")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultTimeout,
		"Timeout for the upload operation")

	_ = cmd.MarkFlagRequired("secret-name")
	_ = cmd.MarkFlagRequired("namespace")
	_ = cmd.MarkFlagRequired("settings-hash")
	_ = cmd.MarkFlagRequired("owner-name")

	return cmd
}

// run executes the keystore upload logic.
func run(ctx context.Context, keystorePath, secretName, namespace, settingsHash, ownerName string) error {
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

	// Get the owner Elasticsearch resource for setting owner references
	var owner *esv1.Elasticsearch
	owner, err = getElasticsearchOwner(ctx, k8sClient, namespace, ownerName)
	if err != nil {
		return fmt.Errorf("failed to get Elasticsearch owner: %w", err)
	}

	// Create or update the Secret using the standard reconciler
	if err := reconcileKeystoreSecret(ctx, k8sClient, secretName, namespace, keystoreData, settingsHash, digest, owner); err != nil {
		return fmt.Errorf("failed to reconcile keystore secret: %w", err)
	}

	log.Info("Successfully uploaded keystore to Secret", "namespace", namespace, "secret", secretName)
	return nil
}

// computeDigest computes the SHA-256 digest of the given data and returns it as a hex string.
func computeDigest(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// createClient creates a new Kubernetes client using in-cluster config.
func createClient() (client.Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	// Register the Elasticsearch scheme
	if err := esv1.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("failed to add Elasticsearch scheme: %w", err)
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return k8sClient, nil
}

// getElasticsearchOwner retrieves the Elasticsearch resource to use as the owner reference.
func getElasticsearchOwner(ctx context.Context, k8sClient client.Client, namespace, name string) (*esv1.Elasticsearch, error) {
	var es esv1.Elasticsearch
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &es); err != nil {
		return nil, err
	}
	return &es, nil
}

// reconcileKeystoreSecret creates or updates the keystore Secret using the standard reconciler.
func reconcileKeystoreSecret(
	ctx context.Context,
	k8sClient k8s.Client,
	secretName, namespace string,
	keystoreData []byte,
	settingsHash, digest string,
	owner *esv1.Elasticsearch,
) error {
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Annotations: map[string]string{
				esv1.KeystoreHashAnnotation:   settingsHash,
				esv1.KeystoreDigestAnnotation: digest,
			},
		},
		Data: map[string][]byte{
			KeystoreFileName: keystoreData,
		},
	}

	// Use the standard ReconcileSecret which handles:
	// - Create vs Update logic
	// - Owner reference setting
	// - Preserving existing labels/annotations
	_, err := reconciler.ReconcileSecret(ctx, k8sClient, expected, owner)
	return err
}
