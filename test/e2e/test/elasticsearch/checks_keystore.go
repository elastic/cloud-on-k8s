// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

func CheckESKeystoreEntries(k *test.K8sClient, b Builder, expectedKeys []string) test.Step {
	return test.Step{
		Name: "Elasticsearch secure settings should eventually be set in all nodes keystore",
		Test: test.Eventually(func() error {
			pods, err := k.GetPods(test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name)...)
			if err != nil {
				return err
			}
			// wait for any ongoing rolling-upgrade to be over
			if err := allPodsReady(b, k); err != nil {
				return err
			}
			if err := clusterHealthGreen(b, k); err != nil {
				return err
			}
			// check keystore entries on all Pods
			if err := test.OnAllPods(pods, func(p corev1.Pod) error {
				// exec into the pod to list keystore entries
				stdout, stderr, err := k.Exec(k8s.ExtractNamespacedName(&p), []string{initcontainer.KeystoreBinPath, "list"})
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
			}); err != nil {
				return err
			}

			return nil
		}),
	}
}
