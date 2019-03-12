// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package snapshot

import (
	"path"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/user"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"

	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// Type represents the component here the snapshotter
	Type = "snapshotter"
	// CertificateLocationVar is the env variable holding a path where ca certs can be found.
	CertificateLocationVar = "CERTIFICATE_LOCATION"
	// UserNameVar is the env variable holding the name of the Es user to be used for snapshots.
	UserNameVar = "USERNAME"
	// UserPasswordVar is the env variable holding the password of the user to be used for snapshots.
	UserPasswordVar = "PASSWORD"
	// EsURLVar is the env variable holding the URL of the Es cluster to take snapshots of.
	EsURLVar = "ELASTICSEARCH_URL"
	// IntervalVar is the env variable specifying the snapshot interval.
	IntervalVar = "INTERVAL"
	// MaxVar is the env variable specifying the maximum number of snapshots to retain.
	MaxVar = "MAX"
)

var cronSchedule = "*/10 * * * *"

// CronJobParams describe parameters to construct a snapshotter job.
type CronJobParams struct {
	Parent types.NamespacedName
	// TODO refactor to just use namespaced Name
	Elasticsearch    v1alpha1.Elasticsearch
	SnapshotterImage string
	User             client.UserAuth
	EsURL            string
}

// CronJobName returns the name of the cronjob for the given parent resource (Elasticsearch).
func CronJobName(parent types.NamespacedName) string {
	return stringsutil.Concat(parent.Name, "-snapshotter")
}

// NewLabels constructs a new set of labels from a Elasticsearch definition.
func NewLabels(es v1alpha1.Elasticsearch) map[string]string {
	var labels = map[string]string{
		label.ClusterNameLabelName: es.Name,
		common.TypeLabelName:       Type,
	}
	return labels
}

// NewCronJob constructor for snapshotter cronjobs.
func NewCronJob(params CronJobParams) *batchv1beta1.CronJob {
	parallelism := int32(1)
	completions := int32(1)
	backoffLimit := int32(0) // don't retry on failure
	caCertSecret := volume.NewSecretVolume(nodecerts.CASecretNameForCluster(params.Elasticsearch.Name), "ca")
	certPath := path.Join(volume.DefaultSecretMountPath, certificates.CAFileName)

	meta := metav1.ObjectMeta{
		Namespace: params.Parent.Namespace,
		Name:      CronJobName(params.Parent),
		Labels:    NewLabels(params.Elasticsearch),
	}

	automountServiceAccountToken := false
	return &batchv1beta1.CronJob{
		ObjectMeta: meta,
		Spec: batchv1beta1.CronJobSpec{
			Schedule:          cronSchedule,
			ConcurrencyPolicy: batchv1beta1.ForbidConcurrent,
			JobTemplate: batchv1beta1.JobTemplateSpec{
				ObjectMeta: meta,
				Spec: batchv1.JobSpec{
					Parallelism:  &parallelism,
					Completions:  &completions,
					BackoffLimit: &backoffLimit,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: meta,
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{{
								Env: []corev1.EnvVar{
									{Name: CertificateLocationVar, Value: certPath},
									{Name: EsURLVar, Value: params.EsURL},
									{Name: UserNameVar, Value: params.User.Name},
									{Name: UserPasswordVar, ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: user.ElasticInternalUsersSecretName(params.Parent.Name),
											},
											Key: params.User.Name,
										},
									}},
								},
								Image: params.SnapshotterImage,
								Args:  []string{"snapshotter"},
								Name:  CronJobName(params.Parent),
								VolumeMounts: []corev1.VolumeMount{
									caCertSecret.VolumeMount(),
								},
							}},
							Volumes: []corev1.Volume{
								caCertSecret.Volume(),
							},
							AutomountServiceAccountToken: &automountServiceAccountToken,
						},
					},
				},
			},
		},
	}
}
