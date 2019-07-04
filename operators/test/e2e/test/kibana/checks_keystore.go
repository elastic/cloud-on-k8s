// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"fmt"
	"reflect"
	"strings"

	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

const (
	keystoreBin = "/usr/share/kibana/bin/kibana-keystore"
)

func CheckKibanaKeystoreEntries(k *test.K8sClient, kb kbtype.Kibana, expectedKeys []string) test.Step {
	return test.Step{
		Name: "Kibana secure settings should eventually be set in all nodes keystore",
		Test: test.Eventually(func() error {
			pods, err := k.GetPods(test.KibanaPodListOptions(kb.Name))
			if err != nil {
				return err
			}
			return test.OnAllPods(pods, func(p corev1.Pod) error {
				// exec into the pod to list keystore entries
				stdout, stderr, err := k.Exec(k8s.ExtractNamespacedName(&p), []string{keystoreBin, "list"})
				if err != nil {
					return errors.Wrap(err, fmt.Sprintf("stdout:\n%s\nstderr:\n%s", stdout, stderr))
				}

				// parse entries from stdout
				var entries []string
				// remove trailing newlines and whitespaces
				trimmed := strings.TrimSpace(stdout)
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
