// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystoreuploader

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// addDiscardLoggerToContext adds a discard logger to the context for testing.
func addDiscardLoggerToContext(ctx context.Context) context.Context {
	return ulog.AddToContext(ctx, logr.Discard())
}

func TestComputeDigest(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "empty data",
			data:     []byte{},
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:     "hello world",
			data:     []byte("hello world"),
			expected: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
		{
			name:     "binary data",
			data:     []byte{0x00, 0x01, 0x02, 0x03},
			expected: "054edec1d0211f624fed0cbca9d4f9400b0e491c43742af2c5b0abebf0c990d8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeDigest(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCommand(t *testing.T) {
	cmd := Command()
	assert.Equal(t, "keystore-uploader", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)

	// Check required flags
	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("keystore-path"))
	assert.NotNil(t, flags.Lookup("secret-name"))
	assert.NotNil(t, flags.Lookup("namespace"))
	assert.NotNil(t, flags.Lookup("settings-hash"))
	assert.NotNil(t, flags.Lookup("timeout"))
}

func TestReconcileStagingSecret_Create(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, esv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	ctx := addDiscardLoggerToContext(context.Background())
	keystoreData := []byte("test keystore data")
	settingsHash := "abc123"
	digest := computeDigest(keystoreData)

	err := reconcileStagingSecret(ctx, k8sClient, "test-secret", "elastic-system", keystoreData, settingsHash, digest)
	require.NoError(t, err)

	// Verify secret was created
	var secret corev1.Secret
	err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "elastic-system", Name: "test-secret"}, &secret)
	require.NoError(t, err)

	assert.Equal(t, keystoreData, secret.Data[KeystoreFileName])
	assert.Equal(t, settingsHash, secret.Annotations[esv1.KeystoreHashAnnotation])
	assert.Equal(t, digest, secret.Annotations[esv1.KeystoreDigestAnnotation])
	// Staging secret has no owner references (cross-namespace)
	assert.Empty(t, secret.OwnerReferences)
}

func TestReconcileStagingSecret_Update(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, esv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "elastic-system",
			Annotations: map[string]string{
				esv1.KeystoreHashAnnotation:   "old-hash",
				esv1.KeystoreDigestAnnotation: "old-digest",
			},
		},
		Data: map[string][]byte{
			KeystoreFileName: []byte("old data"),
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingSecret).
		Build()

	ctx := addDiscardLoggerToContext(context.Background())
	keystoreData := []byte("new keystore data")
	settingsHash := "new-hash"
	digest := computeDigest(keystoreData)

	err := reconcileStagingSecret(ctx, k8sClient, "test-secret", "elastic-system", keystoreData, settingsHash, digest)
	require.NoError(t, err)

	// Verify secret was updated
	var secret corev1.Secret
	err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "elastic-system", Name: "test-secret"}, &secret)
	require.NoError(t, err)

	assert.Equal(t, keystoreData, secret.Data[KeystoreFileName])
	assert.Equal(t, settingsHash, secret.Annotations[esv1.KeystoreHashAnnotation])
	assert.Equal(t, digest, secret.Annotations[esv1.KeystoreDigestAnnotation])
}

func TestRun_FileNotFound(t *testing.T) {
	ctx := context.Background()
	err := run(ctx, "/nonexistent/path/to/keystore", "secret", "namespace", "hash")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read keystore file")
}

func TestRun_EmptyFile(t *testing.T) {
	// Create a temporary empty file
	tmpDir := t.TempDir()
	emptyFile := filepath.Join(tmpDir, "empty.keystore")
	require.NoError(t, os.WriteFile(emptyFile, []byte{}, 0644))

	ctx := context.Background()
	err := run(ctx, emptyFile, "secret", "namespace", "hash")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keystore file is empty")
}
