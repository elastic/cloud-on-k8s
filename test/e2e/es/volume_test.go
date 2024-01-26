// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
)

// TestVolumeEmptyDir tests a manual override of the default persistent storage with emptyDir.
func TestVolumeEmptyDir(t *testing.T) {
	b := elasticsearch.NewBuilder("test-es-explicit-empty-dir").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithEmptyDirVolumes()

	// volume type will be checked in creation steps
	test.Sequence(nil, test.EmptySteps, b).
		RunSequential(t)
}

func TestVolumeRetention(t *testing.T) {
	var dataCheck *elasticsearch.DataIntegrityCheck
	b := elasticsearch.NewBuilder("test-volume-retain-policy").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithVolumeClaimDeletePolicy(esv1.DeleteOnScaledownOnlyPolicy)

	// Create a cluster configured to retain its PVCs and ingest data
	test.Sequence(nil, func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Ingest verification sample data",
				Test: func(t *testing.T) {
					dataCheck = elasticsearch.NewDataIntegrityCheck(k, b)
					require.NoError(t, dataCheck.Init())
				},
			},
		}
	}, b).RunSequential(t)

	// The cluster has now been deleted as part of our usual test step sequence, but PVCs have been retained.
	// Recreate it just this time without retaining the PVCs.

	b2 := b.WithVolumeClaimDeletePolicy(esv1.DeleteOnScaledownAndClusterDeletionPolicy)
	test.Sequence(nil, func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Verify data has been retained after cluster recreation",
				Test: func(t *testing.T) {
					require.NoError(t, dataCheck.Verify())
				},
			},
		}
	}, b2).RunSequential(t)

	// The cluster has now been deleted including its PVCs as evidenced by our usual deletion test step sequence.
}

func TestVolumeMultiDataPath(t *testing.T) {
	b := elasticsearch.NewBuilder("test-es-multi-data-path").
		WithNodeSet(esv1.NodeSet{
			Name: "default",
			Config: &commonv1.Config{
				Data: map[string]interface{}{
					"path.data": "/mnt/data1,/mnt/data2",
				},
			},
			Count: 1,
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					SecurityContext: test.DefaultSecurityContext(),
					Containers: []corev1.Container{
						{
							Name:      esv1.ElasticsearchContainerName,
							Resources: elasticsearch.DefaultResources,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data1",
									MountPath: "/mnt/data1",
								},
								{
									Name:      "data2",
									MountPath: "/mnt/data2",
								},
							},
						},
					},
				}},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "data1",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("2Gi"),
							},
						},
						StorageClassName: ptr.To[string](test.DefaultStorageClass),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "data2",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("2Gi"),
							},
						},
						StorageClassName: ptr.To[string](test.DefaultStorageClass),
					},
				},
			},
		})

	// successful creation should suffice to demonstrate use of multiple volumes
	test.Sequence(nil, test.EmptySteps, b).
		RunSequential(t)
}

// TestVolumeExpansion resizes an existing pvc and ensures Elasticsearch
// correctly reports the resized filesystem.
func TestVolumeExpansion(t *testing.T) {
	// Is there a storage class we can use that supports volume expansion?
	// Otherwise skip this test.
	storageClass, err := getResizeableStorageClass(test.NewK8sClientOrFatal().Client)
	require.NoError(t, err)
	if storageClass == "" {
		t.Skip("No storage class allowing volume expansion found. Skipping the test.")
	}

	b := elasticsearch.NewBuilder("test-volume-expansion").
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithESDataNodes(2, elasticsearch.DefaultResources)
	t.Log(fmt.Sprintf("Using storage class %s to test volume expansion", storageClass))
	patchStorageClasses(&b.Elasticsearch, storageClass)

	masterSset := esv1.StatefulSet(b.Elasticsearch.Name, b.Elasticsearch.Spec.NodeSets[0].Name)
	dataSset := esv1.StatefulSet(b.Elasticsearch.Name, b.Elasticsearch.Spec.NodeSets[1].Name)

	// resize the volume with an additional 1Gi after the cluster is up
	initialStorageSize := b.Elasticsearch.Spec.NodeSets[0].VolumeClaimTemplates[0].Spec.Resources.Requests.Storage()
	resizedStorage := initialStorageSize.DeepCopy()
	resizedStorage.Add(resource.MustParse("1Gi"))

	// Create a copy of the builder with the expected storage resources to use in the regular checks made after updating the Elasticsearch resource
	scaledUpStorage := b.DeepCopy()
	patchStorageSize(&scaledUpStorage.Elasticsearch, resizedStorage)

	test.Sequence(nil, func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Update the Elasticsearch spec with increased storage requests",
				Test: test.Eventually(func() error {
					var es esv1.Elasticsearch
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Elasticsearch), &es); err != nil {
						return err
					}
					patchStorageSize(&es, resizedStorage)
					return k.Client.Update(context.Background(), &es)
				}),
			},
			{
				Name: "StatefulSets should eventually be recreated with the right storage size",
				Test: test.Eventually(func() error {
					for _, ssetName := range []string{masterSset, dataSset} {
						var sset appsv1.StatefulSet
						if err := k.Client.Get(context.Background(), types.NamespacedName{Namespace: b.Elasticsearch.Namespace, Name: ssetName}, &sset); err != nil {
							return err
						}
						if !sset.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests.Storage().Equal(resizedStorage) {
							return fmt.Errorf("StatefulSet %s has not been recreated with storage size %s", ssetName, resizedStorage.String())
						}
					}
					return nil
				}),
			},
			// re-run all the regular checks
		}.WithSteps(test.CheckTestSteps(scaledUpStorage, k))
	}, b).RunSequential(t)
}

func getResizeableStorageClass(k8sClient k8s.Client) (string, error) {
	var scs storagev1.StorageClassList
	if err := k8sClient.List(context.Background(), &scs); err != nil {
		return "", err
	}
	for _, sc := range scs.Items {
		// TODO https://github.com/Azure/AKS/issues/1477 azure-disk does not support resizing of "attached" disks, despite
		// advertising it allows volume expansion. Remove the azure special case once this issue is resolved.
		if sc.AllowVolumeExpansion != nil && *sc.AllowVolumeExpansion && sc.Provisioner != "kubernetes.io/azure-disk" {
			return sc.Name, nil
		}
	}
	// not found
	return "", nil
}

func patchStorageClasses(es *esv1.Elasticsearch, storageClassName string) {
	for nodeSetIndex := range es.Spec.NodeSets {
		for claimIndex := range es.Spec.NodeSets[nodeSetIndex].VolumeClaimTemplates {
			es.Spec.NodeSets[nodeSetIndex].VolumeClaimTemplates[claimIndex].Spec.StorageClassName = ptr.To[string](storageClassName)
		}
	}
}

func patchStorageSize(es *esv1.Elasticsearch, size resource.Quantity) {
	for nodeSetIndex := range es.Spec.NodeSets {
		for claimIndex := range es.Spec.NodeSets[nodeSetIndex].VolumeClaimTemplates {
			es.Spec.NodeSets[nodeSetIndex].VolumeClaimTemplates[claimIndex].Spec.Resources.Requests[corev1.ResourceStorage] = size
		}
	}
}
