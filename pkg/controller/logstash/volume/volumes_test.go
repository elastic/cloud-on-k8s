// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import (
	"testing"

	"github.com/stretchr/testify/require"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_getVolumesFromAssociations(t *testing.T) {
	// Note: we use setAssocConfs to set the AssociationConfs which are normally set in the reconciliation loop.
	for _, tt := range []struct {
		name                   string
		logstash               logstashv1alpha1.Logstash
		setAssocConfs          func(assocs []commonv1.Association)
		wantAssociationsLength int
	}{
		{
			name: "es refs",
			logstash: logstashv1alpha1.Logstash{
				Spec: logstashv1alpha1.LogstashSpec{
					ElasticsearchRefs: []logstashv1alpha1.ElasticsearchCluster{
						{
							ObjectSelector: commonv1.ObjectSelector{Name: "elasticsearch"},
							ClusterName:    "production",
						},
						{
							ObjectSelector: commonv1.ObjectSelector{Name: "elasticsearch2"},
							ClusterName:    "production2",
						},
					},
				},
			},
			setAssocConfs: func(assocs []commonv1.Association) {
				assocs[0].SetAssociationConf(&commonv1.AssociationConf{
					CASecretName: "elasticsearch-es-ca",
				})
				assocs[1].SetAssociationConf(&commonv1.AssociationConf{
					CASecretName: "elasticsearch2-es-ca",
				})
			},
			wantAssociationsLength: 2,
		},
		{
			name: "one es ref with ca, another no ca",
			logstash: logstashv1alpha1.Logstash{
				Spec: logstashv1alpha1.LogstashSpec{
					ElasticsearchRefs: []logstashv1alpha1.ElasticsearchCluster{
						{
							ObjectSelector: commonv1.ObjectSelector{Name: "uat"},
							ClusterName:    "uat",
						},
						{
							ObjectSelector: commonv1.ObjectSelector{Name: "production"},
							ClusterName:    "production",
						},
					},
				},
			},
			setAssocConfs: func(assocs []commonv1.Association) {
				assocs[0].SetAssociationConf(&commonv1.AssociationConf{
					// No CASecretName
				})
				assocs[1].SetAssociationConf(&commonv1.AssociationConf{
					CASecretName: "production-es-ca",
				})
			},
			wantAssociationsLength: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assocs := tt.logstash.GetAssociations()
			tt.setAssocConfs(assocs)
			associations, err := getVolumesFromAssociations(assocs)
			require.NoError(t, err)
			require.Equal(t, tt.wantAssociationsLength, len(associations))
		})
	}
}

func Test_BuildVolumesAndMounts(t *testing.T) {
	hostPathType := corev1.HostPathDirectoryOrCreate

	tt := []struct {
		name     string
		logstash logstashv1alpha1.Logstash
		useTLS   bool
	}{
		{
			name: "with default data PVC",
			logstash: logstashv1alpha1.Logstash{
				Spec: logstashv1alpha1.LogstashSpec{},
			},
			useTLS: false,
		},
		{
			name: "with default data PVC and http certs",
			logstash: logstashv1alpha1.Logstash{
				Spec: logstashv1alpha1.LogstashSpec{},
			},
			useTLS: true,
		},
		{
			name: "with user provided data PVC",
			logstash: logstashv1alpha1.Logstash{
				Spec: logstashv1alpha1.LogstashSpec{
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "logstash-data",
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteOnce,
								},
								Resources: corev1.VolumeResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("42Ti"),
									},
								},
							},
						},
					}},
			},
			useTLS: false,
		},
		{
			name: "with user provided other PVC",
			logstash: logstashv1alpha1.Logstash{
				Spec: logstashv1alpha1.LogstashSpec{
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pq",
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteOnce,
								},
								Resources: corev1.VolumeResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("42Ti"),
									},
								},
							},
						},
					}},
			},
			useTLS: false,
		},
		{
			name: "with user provided other PVC and logstash-data",
			logstash: logstashv1alpha1.Logstash{
				Spec: logstashv1alpha1.LogstashSpec{
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pq",
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteOnce,
								},
								Resources: corev1.VolumeResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("42Ti"),
									},
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "logstash-data",
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteOnce,
								},
								Resources: corev1.VolumeResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("42Ti"),
									},
								},
							},
						},
					}},
			},
			useTLS: false,
		},
		{
			name: "with user provided data empty volume",
			logstash: logstashv1alpha1.Logstash{
				Spec: logstashv1alpha1.LogstashSpec{
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Volumes: []corev1.Volume{{
								Name: "logstash-data",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								}},
							},
						},
					},
				},
			},
			useTLS: false,
		},
		{
			name: "with user provided data hostpath volume",
			logstash: logstashv1alpha1.Logstash{
				Spec: logstashv1alpha1.LogstashSpec{

					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Volumes: []corev1.Volume{{
								Name: "logstash-data",
								VolumeSource: corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{
										Path: "/mnt/data",
										Type: &hostPathType,
									},
								},
							}},
						},
					},
				},
			},
			useTLS: false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			tc.logstash.Spec.VolumeClaimTemplates = AppendDefaultPVCs(tc.logstash.Spec.VolumeClaimTemplates,
				tc.logstash.Spec.PodTemplate.Spec)
			_, volumeMounts, err := BuildVolumes(tc.logstash, tc.useTLS)
			assert.NoError(t, err)
			assert.True(t, contains(volumeMounts, "logstash-data", "/usr/share/logstash/data"))
			assert.True(t, contains(volumeMounts, "logstash-logs", "/usr/share/logstash/logs"))
			assert.True(t, contains(volumeMounts, "config", "/usr/share/logstash/config"))
			if tc.useTLS {
				assert.True(t, contains(volumeMounts, "elastic-internal-http-certificates", "/mnt/elastic-internal/http-certs"))
			}
		})
	}
}

func contains(volumeMounts []corev1.VolumeMount, volumeMountName, volumeMountPath string) bool {
	for _, vm := range volumeMounts {
		if vm.Name == volumeMountName && vm.MountPath == volumeMountPath {
			return true
		}
	}
	return false
}
