// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"context"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	corev1 "k8s.io/api/core/v1"
)

func AnnotatePodsWithBuilderHash(subj, prev Subject, k *K8sClient) StepList {
	return []Step{
		{
			Name: "Annotate Pods with a hash of their Builder spec",
			Test: Eventually(func() error {
				var pods corev1.PodList
				if err := k.Client.List(context.Background(), &pods, subj.ListOptions()...); err != nil {
					return err
				}

				expectedHash := hash.HashObject(prev.Spec())
				for _, pod := range pods.Items {
					if err := AnnotatePodWithBuilderHash(k, pod, expectedHash); err != nil {
						return err
					}
				}
				return nil
			}),
		},
	}
}
