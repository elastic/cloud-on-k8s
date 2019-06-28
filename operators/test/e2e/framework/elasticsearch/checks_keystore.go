// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

func CheckESKeystoreEntries(k *framework.K8sClient, es v1alpha1.Elasticsearch, expectedKeys []string) framework.TestStep {
	return framework.TestStep{
		Name: "Elasticsearch secure settings should eventually be set in all nodes keystore",
		Test: framework.Eventually(func() error {
			pods, err := k.GetPods(framework.ESPodListOptions(es.Name))
			if err != nil {
				return err
			}
			return framework.OnAllPods(pods, func(p corev1.Pod) error {
				// exec into the pod to list keystore entries
				stdout, stderr, err := k.Exec(k8s.ExtractNamespacedName(&p), []string{keystore.KeystoreBinPath, "list"})
				if err != nil {
					return errors.Wrap(err, fmt.Sprintf("stdout:\n%s\nstderr:\n%s", stdout, stderr))
				}

				// parse entries from stdout
				var entries []string
				// the keystore contains a "keystore.seed" entry we don't want to include in the comparison
				noKeystoreSeeds := strings.Replace(stdout, "keystore.seed\n", "", 1)
				// remove trailing newlines and whitespaces
				trimmed := strings.TrimSpace(noKeystoreSeeds)
				// split by lines, unless no output
				if trimmed != "" {
					entries = strings.Split(trimmed, "\n")
				}

				if !reflect.DeepEqual(expectedKeys, entries) {
					return fmt.Errorf("invalid keystore entries. Expected: %s. Actual: %s", expectedKeys, entries)
				}
				return nil
			})
		}),
	}
}
