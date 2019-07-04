// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultRetryDelay = 3 * time.Second
	defaultTimeout    = 5 * time.Minute
)

func CheckKeystoreEntries(k *K8sClient, listOption client.ListOptions, KeystoreCmd []string, expectedKeys []string) Step {
	return Step{
		Name: "Kibana secure settings should eventually be set in all nodes keystore",
		Test: Eventually(func() error {
			pods, err := k.GetPods(listOption)
			if err != nil {
				return err
			}
			return OnAllPods(pods, func(p corev1.Pod) error {
				// exec into the pod to list keystore entries
				stdout, stderr, err := k.Exec(k8s.ExtractNamespacedName(&p), append(KeystoreCmd, "list"))
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

// ExitOnErr exits with code 1 if the given error is not nil
func ExitOnErr(err error) {
	if err != nil {
		fmt.Println(err)
		fmt.Println("Exiting.")
		os.Exit(1)
	}
}

// Eventually runs the given function until success,
// with a default timeout
func Eventually(f func() error) func(*testing.T) {
	return func(t *testing.T) {
		fmt.Printf("Retries (%s timeout): ", defaultTimeout)
		err := retry.UntilSuccess(func() error {
			fmt.Print(".") // super modern progress bar 2.0!
			return f()
		}, defaultTimeout, DefaultRetryDelay)
		fmt.Println()
		require.NoError(t, err)
	}
}

// BoolPtr returns a pointer to a bool/
func BoolPtr(b bool) *bool {
	return &b
}
