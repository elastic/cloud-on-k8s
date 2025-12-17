// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package keystorejob provides functionality to reconcile a Kubernetes Job that creates
// an Elasticsearch keystore file and uploads it to a Secret for the reloadable keystore
// feature available in Elasticsearch 9.3+.
package keystorejob

import (
	"bytes"
	"context"
	"text/template"

	"go.elastic.co/apm/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	// KeystoreVolumeName is the name of the emptyDir volume shared between init and main container.
	KeystoreVolumeName = "keystore-volume"
	// KeystoreVolumeMountPath is where the keystore file is written by the init container.
	KeystoreVolumeMountPath = "/mnt/elastic-internal/keystore"
	// KeystoreFileName is the name of the keystore file.
	KeystoreFileName = "elasticsearch.keystore"

	// Job configuration
	jobBackoffLimit            = int32(3)
	jobActiveDeadlineSeconds   = int64(900) // 15 minutes
	jobTTLSecondsAfterFinished = int32(300) // 5 minutes
)

// MinVersion is the minimum Elasticsearch version that supports the reloadable keystore feature.
// This feature requires the enhanced _nodes/reload_secure_settings API that returns keystore digests.
var MinVersion = version.MinFor(9, 3, 0)

// ShouldUseReloadableKeystore returns true if the reloadable keystore feature should be used
// for the given Elasticsearch cluster.
//
// Requirements for the feature to be enabled:
// - Elasticsearch version 9.3.0 or later
// - The feature is not explicitly disabled via annotation
// - Secure settings exist (determined by keystoreResources being non-nil)
// - The operator image is configured (required for the keystore job)
//
// The keystoreResources parameter should come from keystore.ReconcileResources which aggregates
// all secure settings sources (spec, StackConfigPolicy, remote cluster keys, etc.).
//
// When the feature is not used, the reason is logged at the appropriate level.
func ShouldUseReloadableKeystore(ctx context.Context, es esv1.Elasticsearch, esVersion version.Version, keystoreResources *keystore.Resources, operatorImage string) bool {
	log := ulog.FromContext(ctx)

	if es.IsReloadableKeystoreDisabled() {
		log.V(1).Info("Reloadable keystore disabled via annotation",
			"namespace", es.Namespace, "es_name", es.Name)
		return false
	}
	if !esVersion.GTE(MinVersion) {
		// Not logging - this is the normal case for older versions
		return false
	}
	// keystoreResources is nil when there are no secure settings from any source
	if keystoreResources == nil {
		// Not logging - no secure settings means nothing to do
		return false
	}
	// operatorImage is required for the keystore job's uploader container
	if operatorImage == "" {
		log.Info("Reloadable keystore feature requires --operator-image flag to be set, falling back to init container approach",
			"namespace", es.Namespace, "es_name", es.Name)
		return false
	}
	return true
}

const (
	// KeystoreUploaderServiceAccount is the name of the service account used by keystore jobs.
	// This service account is created by the Helm chart in the operator namespace with
	// minimal permissions (only secrets in the operator namespace).
	KeystoreUploaderServiceAccount = "eck-keystore-uploader"
)

// Params holds the parameters for reconciling the keystore Job.
type Params struct {
	ES                 esv1.Elasticsearch
	Client             k8s.Client
	OperatorNamespace  string
	OperatorImage      string
	ElasticsearchImage string
	// KeystoreResources comes from keystore.ReconcileResources and contains
	// the aggregated secure settings volume and hash from all sources.
	KeystoreResources *keystore.Resources
	Meta              metadata.Metadata
	// PodTemplate contains settings to inherit from the ES pod template for the Job's pods.
	PodTemplate JobPodTemplateParams
}

// JobPodTemplateParams contains pod-level settings to apply to the keystore Job's pods.
// These are typically inherited from the Elasticsearch pod template to ensure the Job
// can run in the same environment (e.g., private registries, pod security policies).
type JobPodTemplateParams struct {
	// ImagePullSecrets for pulling the Elasticsearch image in the init container.
	ImagePullSecrets []corev1.LocalObjectReference
	// PodSecurityContext to apply to the Job pods.
	PodSecurityContext *corev1.PodSecurityContext
}

// ReconcileJob reconciles the keystore creation Job for Elasticsearch 9.3+ clusters.
// The job runs in the operator namespace and creates a "staging" Secret there.
// This function then copies the staging Secret to the ES namespace once the job completes.
//
// It returns:
//   - done: true if the keystore Secret is ready (job completed successfully and copied)
//   - err: any error encountered during reconciliation
func ReconcileJob(ctx context.Context, params Params) (done bool, err error) {
	span, ctx := apm.StartSpan(ctx, "reconcile_keystore_job", tracing.SpanTypeApp)
	defer span.End()

	log := ulog.FromContext(ctx)
	es := params.ES

	// Names for resources in the operator namespace (staging area)
	stagingSecretName := esv1.StagingKeystoreSecretName(es.Namespace, es.Name)
	jobName := esv1.KeystoreJobName(es.Namespace, es.Name)

	// Name for the final secret in the ES namespace
	finalSecretName := esv1.KeystoreSecretName(es.Name)

	secureSettingsHash := params.KeystoreResources.Hash

	// Check if the final keystore Secret already exists with the correct hash
	var finalSecret corev1.Secret
	err = params.Client.Get(ctx, types.NamespacedName{Namespace: es.Namespace, Name: finalSecretName}, &finalSecret)
	if err != nil && !apierrors.IsNotFound(err) {
		return false, err
	}

	if err == nil {
		// Final secret exists - check if it has the correct hash
		existingHash := finalSecret.Annotations[esv1.KeystoreHashAnnotation]
		if existingHash == secureSettingsHash {
			log.V(1).Info("Keystore secret already up to date", "hash", existingHash)
			// Clean up staging secret if it exists
			_ = deleteStagingSecret(ctx, params.Client, params.OperatorNamespace, stagingSecretName)
			return true, nil
		}
		log.Info("Keystore secret hash mismatch, need to recreate", "expected", secureSettingsHash, "actual", existingHash)
	}

	// Check if staging Secret exists (job completed)
	var stagingSecret corev1.Secret
	err = params.Client.Get(ctx, types.NamespacedName{Namespace: params.OperatorNamespace, Name: stagingSecretName}, &stagingSecret)
	if err != nil && !apierrors.IsNotFound(err) {
		return false, err
	}

	// Handle existing staging secret
	if err == nil {
		done, err := handleStagingSecret(ctx, params, &stagingSecret, finalSecretName, secureSettingsHash)
		if done || err != nil {
			return done, err
		}
	}

	// Check if a Job already exists (in operator namespace)
	var existingJob batchv1.Job
	err = params.Client.Get(ctx, types.NamespacedName{Namespace: params.OperatorNamespace, Name: jobName}, &existingJob)
	if err != nil && !apierrors.IsNotFound(err) {
		return false, err
	}

	jobExists := err == nil
	if jobExists {
		if err := handleExistingJob(ctx, params, &existingJob, secureSettingsHash); err != nil {
			return false, err
		}
		return false, nil
	}

	// Create a new Job in the operator namespace
	job := buildJob(params, secureSettingsHash)
	log.Info("Creating keystore job", "job", jobName, "namespace", params.OperatorNamespace, "hash", secureSettingsHash)
	if err := params.Client.Create(ctx, &job); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Race condition - job was created by another reconciliation
			return false, nil
		}
		return false, err
	}

	return false, nil
}

// copyStagingSecretToESNamespace copies the staging secret from the operator namespace
// to the ES namespace, setting the owner reference to the ES resource.
func copyStagingSecretToESNamespace(ctx context.Context, params Params, stagingSecret *corev1.Secret, targetName string) error {
	log := ulog.FromContext(ctx)
	es := params.ES

	finalSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetName,
			Namespace: es.Namespace,
			Labels:    params.Meta.Labels,
			Annotations: map[string]string{
				esv1.KeystoreHashAnnotation:   stagingSecret.Annotations[esv1.KeystoreHashAnnotation],
				esv1.KeystoreDigestAnnotation: stagingSecret.Annotations[esv1.KeystoreDigestAnnotation],
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         esv1.GroupVersion.String(),
					Kind:               esv1.Kind,
					Name:               es.Name,
					UID:                es.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				},
			},
		},
		Data: stagingSecret.Data,
	}

	// Check if final secret already exists
	var existing corev1.Secret
	err := params.Client.Get(ctx, types.NamespacedName{Namespace: es.Namespace, Name: targetName}, &existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create new secret
			log.Info("Creating keystore secret from staging", "namespace", es.Namespace, "secret", targetName)
			return params.Client.Create(ctx, &finalSecret)
		}
		return err
	}

	// Update existing secret
	existing.Data = finalSecret.Data
	existing.Annotations = finalSecret.Annotations
	existing.OwnerReferences = finalSecret.OwnerReferences
	log.Info("Updating keystore secret from staging", "namespace", es.Namespace, "secret", targetName)
	return params.Client.Update(ctx, &existing)
}

// deleteStagingSecret deletes the staging secret from the operator namespace.
func deleteStagingSecret(ctx context.Context, client k8s.Client, namespace, name string) error {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := client.Delete(ctx, &secret)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// handleStagingSecret handles the case where a staging secret exists in the operator namespace.
// Returns (true, nil) if the secret was successfully copied to the ES namespace.
// Returns (false, nil) if the secret was deleted due to hash mismatch.
// Returns (false, err) on error.
func handleStagingSecret(ctx context.Context, params Params, stagingSecret *corev1.Secret, finalSecretName, secureSettingsHash string) (bool, error) {
	log := ulog.FromContext(ctx)
	stagingSecretName := stagingSecret.Name

	stagingHash := stagingSecret.Annotations[esv1.KeystoreHashAnnotation]
	if stagingHash != secureSettingsHash {
		// Staging secret has wrong hash - delete it
		log.Info("Deleting stale staging secret", "oldHash", stagingHash, "newHash", secureSettingsHash)
		if err := deleteStagingSecret(ctx, params.Client, params.OperatorNamespace, stagingSecretName); err != nil {
			return false, err
		}
		return false, nil
	}

	// Copy staging secret to ES namespace
	if err := copyStagingSecretToESNamespace(ctx, params, stagingSecret, finalSecretName); err != nil {
		return false, err
	}
	// Delete staging secret
	if err := deleteStagingSecret(ctx, params.Client, params.OperatorNamespace, stagingSecretName); err != nil {
		log.Error(err, "Failed to delete staging secret", "secret", stagingSecretName)
		// Don't fail - the secret was copied successfully
	}
	return true, nil
}

// handleExistingJob handles the case where a keystore Job already exists in the operator namespace.
// It checks if the job is for the current hash and handles completion, failure, or running states.
func handleExistingJob(ctx context.Context, params Params, existingJob *batchv1.Job, secureSettingsHash string) error {
	log := ulog.FromContext(ctx)

	existingJobHash := existingJob.Annotations[esv1.KeystoreHashAnnotation]
	if existingJobHash != secureSettingsHash {
		// Job is for a different hash - delete it
		log.Info("Deleting stale keystore job", "oldHash", existingJobHash, "newHash", secureSettingsHash)
		if err := params.Client.Delete(ctx, existingJob); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		// Will recreate on next reconciliation
		return nil
	}

	// Job is for the current hash - check its status
	if isJobComplete(existingJob) {
		// Job completed - the staging secret should now exist
		// Return to let the main reconciliation loop handle copying it
		log.V(1).Info("Keystore job completed, staging secret should be ready")
		return nil
	}

	if isJobFailed(existingJob) {
		log.Info("Keystore job failed, deleting for retry")
		if err := params.Client.Delete(ctx, existingJob); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		// Will recreate on next reconciliation
		return nil
	}

	// Job is still running
	log.V(1).Info("Keystore job still running")
	return nil
}

// buildJob creates the Job spec for creating the keystore file.
// The job runs in the operator namespace and creates a staging Secret there.
func buildJob(params Params, secureSettingsHash string) batchv1.Job {
	es := params.ES

	// Job and staging secret are in the operator namespace, names include ES namespace to avoid collisions
	jobName := esv1.KeystoreJobName(es.Namespace, es.Name)
	stagingSecretName := esv1.StagingKeystoreSecretName(es.Namespace, es.Name)

	// Labels for the job - don't use ES labels like cluster-name to avoid
	// other controllers (e.g. license controller) picking up job pods.
	// The job name already includes the ES namespace and name for identification.
	labels := map[string]string{
		"app.kubernetes.io/name":       "eck-keystore-job",
		"app.kubernetes.io/managed-by": "eck-operator",
	}
	annotations := map[string]string{
		esv1.KeystoreHashAnnotation: secureSettingsHash,
	}

	// Build init container using the existing keystore init container logic
	initContainer := buildInitContainer(params, secureSettingsHash)

	// Build main container that uploads the keystore to a staging Secret in operator namespace
	mainContainer := buildMainContainer(params, stagingSecretName, secureSettingsHash)

	return batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: params.OperatorNamespace,
			Labels:    labels,
			// No owner reference - job is in operator namespace, ES is in a different namespace
			// Job will be cleaned up by TTLSecondsAfterFinished
			Annotations: annotations,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To(jobBackoffLimit),
			ActiveDeadlineSeconds:   ptr.To(jobActiveDeadlineSeconds),
			TTLSecondsAfterFinished: ptr.To(jobTTLSecondsAfterFinished),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					InitContainers: []corev1.Container{
						initContainer,
					},
					Containers: []corev1.Container{
						mainContainer,
					},
					Volumes: []corev1.Volume{
						// SecureSettings volume from user-provided secrets (from keystore.Resources)
						params.KeystoreResources.Volume,
						// EmptyDir to hold the keystore file
						{
							Name: KeystoreVolumeName,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					// Inherit from ES pod template for private registries
					ImagePullSecrets: params.PodTemplate.ImagePullSecrets,
					// Use the dedicated keystore uploader service account
					ServiceAccountName: KeystoreUploaderServiceAccount,
					// Pod security context for PSS compliance
					SecurityContext: params.PodTemplate.PodSecurityContext,
					// Mount the service account token for API access
					AutomountServiceAccountToken: ptr.To(true),
				},
			},
		},
	}
}

// keystoreInitScript is the bash script to create the keystore file.
// It creates the keystore and adds all entries from the secure settings volume.
// ES_PATH_CONF is set to the output directory so elasticsearch-keystore creates
// the keystore file there instead of the default /usr/share/elasticsearch/config.
const keystoreInitScript = `#!/usr/bin/env bash

set -eux

echo "Initializing keystore."

# Set ES_PATH_CONF to the output directory so the keystore is created there
export ES_PATH_CONF={{.OutputDir}}

# create a keystore in the ES_PATH_CONF directory
{{.KeystoreBin}} create

# add all existing secret entries into it
for filename in {{.SecureSettingsDir}}/*; do
	[[ -e "$filename" ]] || continue # glob does not match
	key=$(basename "$filename")
	echo "Adding "$key" to the keystore."
	{{.KeystoreBin}} add-file "$key" "$filename"
done

# Make the keystore file readable by the uploader container which runs as a different user
chmod 644 {{.OutputDir}}/elasticsearch.keystore

echo "Keystore initialization successful."
`

type keystoreScriptParams struct {
	KeystoreBin       string
	OutputDir         string
	SecureSettingsDir string
}

var keystoreScriptTemplate = template.Must(template.New("keystore-init").Parse(keystoreInitScript))

// buildInitContainer creates the init container that creates the keystore file.
func buildInitContainer(params Params, _ string) corev1.Container {
	privileged := false

	// Generate the script with the correct paths
	var scriptBuf bytes.Buffer
	_ = keystoreScriptTemplate.Execute(&scriptBuf, keystoreScriptParams{
		KeystoreBin:       initcontainer.KeystoreBinPath,
		OutputDir:         KeystoreVolumeMountPath,
		SecureSettingsDir: keystore.SecureSettingsVolumeMountPath,
	})
	script := scriptBuf.String()

	return corev1.Container{
		Name:            keystore.InitContainerName,
		Image:           params.ElasticsearchImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
		},
		Command: []string{"/usr/bin/env", "bash", "-c", script},
		VolumeMounts: []corev1.VolumeMount{
			// Access secure settings - mount the volume from keystore.Resources
			{
				Name:      params.KeystoreResources.Volume.Name,
				MountPath: keystore.SecureSettingsVolumeMountPath,
				ReadOnly:  true,
			},
			// Write keystore to this volume
			{
				Name:      KeystoreVolumeName,
				MountPath: KeystoreVolumeMountPath,
			},
		},
		Resources: initcontainer.KeystoreParams.Resources,
	}
}

// buildMainContainer creates the main container that uploads the keystore to a staging Secret
// in the operator namespace.
func buildMainContainer(params Params, stagingSecretName, secureSettingsHash string) corev1.Container {
	keystorePath := KeystoreVolumeMountPath + "/" + KeystoreFileName
	privileged := false

	return corev1.Container{
		Name:            "keystore-uploader",
		Image:           params.OperatorImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command: []string{
			"/elastic-operator",
			"keystore-uploader",
			"--keystore-path", keystorePath,
			"--secret-name", stagingSecretName,
			"--namespace", params.OperatorNamespace, // Staging secret goes to operator namespace
			"--settings-hash", secureSettingsHash,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      KeystoreVolumeName,
				MountPath: KeystoreVolumeMountPath,
				ReadOnly:  true,
			},
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: initcontainer.KeystoreParams.Resources.Requests[corev1.ResourceMemory],
				corev1.ResourceCPU:    initcontainer.KeystoreParams.Resources.Requests[corev1.ResourceCPU],
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: initcontainer.KeystoreParams.Resources.Limits[corev1.ResourceMemory],
				corev1.ResourceCPU:    initcontainer.KeystoreParams.Resources.Limits[corev1.ResourceCPU],
			},
		},
	}
}

// isJobComplete returns true if the Job has completed successfully.
func isJobComplete(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// isJobFailed returns true if the Job has failed.
func isJobFailed(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// DeleteJob deletes the keystore Job from the operator namespace if it exists.
func DeleteJob(ctx context.Context, client k8s.Client, operatorNamespace string, es esv1.Elasticsearch) error {
	jobName := esv1.KeystoreJobName(es.Namespace, es.Name)
	job := batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: operatorNamespace,
		},
	}
	if err := client.Delete(ctx, &job); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// DeleteKeystoreSecret deletes the keystore Secret from the ES namespace if it exists.
func DeleteKeystoreSecret(ctx context.Context, client k8s.Client, es esv1.Elasticsearch) error {
	secretName := esv1.KeystoreSecretName(es.Name)
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: es.Namespace,
		},
	}
	if err := client.Delete(ctx, &secret); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// DeleteStagingKeystoreSecret deletes the staging keystore Secret from the operator namespace if it exists.
func DeleteStagingKeystoreSecret(ctx context.Context, client k8s.Client, operatorNamespace string, es esv1.Elasticsearch) error {
	secretName := esv1.StagingKeystoreSecretName(es.Namespace, es.Name)
	return deleteStagingSecret(ctx, client, operatorNamespace, secretName)
}
