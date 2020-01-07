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

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultRetryDelay = 3 * time.Second
)

func CheckKeystoreEntries(k *K8sClient, keystoreCmd []string, expectedKeys []string, opts ...client.ListOption) Step {
	return Step{
		Name: "secure settings should eventually be set in all nodes keystore",
		Test: Eventually(func() error {
			pods, err := k.GetPods(opts...)
			if err != nil {
				return err
			}
			return OnAllPods(pods, func(p corev1.Pod) error {
				// exec into the pod to list keystore entries
				stdout, stderr, err := k.Exec(k8s.ExtractNamespacedName(&p), append(keystoreCmd, "list"))
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

// Eventually runs the given function until success with a default timeout.
func Eventually(f func() error) func(*testing.T) {
	return UntilSuccess(f, ctx.TestTimeout)
}

// UntilSuccess executes f until it succeeds, or the timeout is reached.
func UntilSuccess(f func() error, timeout time.Duration) func(*testing.T) {
	return func(t *testing.T) {
		fmt.Printf("Retries (%s timeout): ", timeout)
		err := retry.UntilSuccess(func() error {
			fmt.Print(".") // super modern progress bar 2.0!
			return f()
		}, timeout, DefaultRetryDelay)
		fmt.Println()
		require.NoError(t, err)
	}
}

// BoolPtr returns a pointer to a bool/
func BoolPtr(b bool) *bool {
	return &b
}

// AnnotatePodWithBuilderHash annotates pod with a hash to facilitate detection of newly created pods
func AnnotatePodWithBuilderHash(k *K8sClient, pod corev1.Pod, hash string) error {
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[BuilderHashAnnotation] = hash
	if err := k.Client.Update(&pod); err != nil {
		// may error out with a conflict if concurrently updated by the operator,
		// which is why we retry with `test.Eventually`
		return err
	}

	return nil
}

// ValidateBuilderHashAnnotation validates builder hash. Pod should either:
// - be annotated with the hash of the current spec from previous E2E steps
// - not be annotated at all (if recreated/upgraded, or not a mutation)
// But **not** be annotated with the hash of a different ES spec, meaning
// it probably still matches the spec of the pre-mutation builder (rolling upgrade not over).
//
// Important: this does not catch rolling upgrades due to a keystore change, where the Builder hash
// would stay the same.
func ValidateBuilderHashAnnotation(pod corev1.Pod, hash string) error {
	if pod.Annotations[BuilderHashAnnotation] != "" && pod.Annotations[BuilderHashAnnotation] != hash {
		return fmt.Errorf("pod %s was not upgraded (yet?) to match the expected specification", pod.Name)
	}
	return nil
}
