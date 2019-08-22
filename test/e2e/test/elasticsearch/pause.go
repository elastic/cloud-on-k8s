// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"fmt"
	"strconv"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

func togglePauseOn(paused bool, es v1alpha1.Elasticsearch, k *test.K8sClient) test.Step {
	return test.Step{
		Name: fmt.Sprintf("Should pause reconciliation %v", paused),
		Test: test.Eventually(func() error {
			var curr v1alpha1.Elasticsearch
			if err := k.Client.Get(k8s.ExtractNamespacedName(&es), &curr); err != nil {
				return err
			}
			as := curr.Annotations
			if as == nil {
				as = map[string]string{}
			}
			as[common.PauseAnnotationName] = strconv.FormatBool(paused)
			curr.Annotations = as
			return k.Client.Update(&curr)
		}),
	}
}

func PauseReconciliation(es v1alpha1.Elasticsearch, k *test.K8sClient) test.Step {
	return togglePauseOn(true, es, k)
}

func ResumeReconciliation(es v1alpha1.Elasticsearch, k *test.K8sClient) test.Step {
	return togglePauseOn(false, es, k)
}
