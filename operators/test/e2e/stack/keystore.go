// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

func CheckKeystoreEntries(k *helpers.K8sHelper, es v1alpha1.Elasticsearch, expectedKeys []string) helpers.TestStep {
	return helpers.TestStep{
		Name: "Secure settings should eventually be set in all nodes keystore",
		Test: helpers.Eventually(func() error {
			pods, err := k.GetPods(helpers.ESPodListOptions(es.Name))
			if err != nil {
				return err
			}
			return onAllPods(pods, func(p corev1.Pod) error {
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

func onAllPods(pods []corev1.Pod, f func(corev1.Pod) error) error {
	// map phase: execute a function on all pods in parallel
	fResults := make(chan error, len(pods))
	for _, p := range pods {
		go func(pod corev1.Pod) {
			fResults <- f(pod)
		}(p)
	}
	// reduce phase: aggregate errors (simply return the last one seen)
	var err error
	for range pods {
		podErr := <-fResults
		if podErr != nil {
			err = podErr
		}
	}
	return err
}
