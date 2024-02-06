// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package logstash

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	lsv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/logstash"
)


// TestVolumeExpansion resizes an existing pvc and ensures Logstash
// correctly reports the resized filesystem.
func TestVolumeExpansionLogstash(t *testing.T) {
	// Is there a storage class we can use that supports volume expansion?
	// Otherwise skip this test.
	storageClass, err := getResizeableStorageClass(test.NewK8sClientOrFatal().Client)
	require.NoError(t, err)
	if storageClass == "" {
		t.Skip("No storage class allowing volume expansion found. Skipping the test.")
	}

	b := logstash.NewBuilder("test-volume-expansion")
	t.Log(fmt.Sprintf("Using storage class %s to test volume expansion", storageClass))
	patchStorageClasses(&b.Logstash, storageClass)

	ssetName := lsv1.Name(b.Logstash.Name)


	// resize the volume with an additional 1Gi after the cluster is up
	initialStorageSize := b.Logstash.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests.Storage()
	resizedStorage := initialStorageSize.DeepCopy()
	resizedStorage.Add(resource.MustParse("1Gi"))

	// Create a copy of the builder with the expected storage resources to use in the regular checks made after updating the Elasticsearch resource
	scaledUpStorage := b.DeepCopy()
	patchStorageSize(&scaledUpStorage.Logstash, resizedStorage)

	test.Sequence(nil, func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Update the Logstash spec with increased storage requests",
				Test: test.Eventually(func() error {
					var ls lsv1.Logstash
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Logstash), &ls); err != nil {
						return err
					}
					patchStorageSize(&ls, resizedStorage)
					return k.Client.Update(context.Background(), &ls)
				}),
			},
			{
				Name: "StatefulSets should eventually be recreated with the right storage size",
				Test: test.Eventually(func() error {
					var sset appsv1.StatefulSet
					if err := k.Client.Get(context.Background(), types.NamespacedName{Namespace: b.Logstash.Namespace, Name: ssetName}, &sset); err != nil {
						return err
					}
					if !sset.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests.Storage().Equal(resizedStorage) {
						return fmt.Errorf("StatefulSet %s has not been recreated with storage size %s", ssetName, resizedStorage.String())
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

func patchStorageClasses(ls *lsv1.Logstash, storageClassName string) {
	for claimIndex := range ls.Spec.VolumeClaimTemplates {
		ls.Spec.VolumeClaimTemplates[claimIndex].Spec.StorageClassName = pointer.String(storageClassName)
	}
}

func patchStorageSize(ls *lsv1.Logstash, size resource.Quantity) {
	for claimIndex := range ls.Spec.VolumeClaimTemplates {
		ls.Spec.VolumeClaimTemplates[claimIndex].Spec.Resources.Requests[corev1.ResourceStorage] = size
	}
}