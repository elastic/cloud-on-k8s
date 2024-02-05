// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	corev1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/retry"
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
		t.Helper()
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
	if err := k.Client.Update(context.Background(), &pod); err != nil {
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

// LabelTestPods labels:
// - operator pod,
// - e2e runner pod
func LabelTestPods(c k8s.Client, ctx Context, key, value string) error {
	// label operator pod
	if err := labelPod(
		c,
		ctx.Operator.Name+"-0",
		ctx.Operator.Namespace,
		key,
		value); err != nil {
		return err
	}

	// find and label E2E test runner pod
	podList := corev1.PodList{}
	ns := client.InNamespace(ctx.E2ENamespace)
	if err := c.List(context.Background(), &podList, ns); err != nil {
		return err
	}

	for _, pod := range podList.Items {
		if strings.HasPrefix(pod.Name, "eck-"+ctx.TestRun) {
			return labelPod(
				c,
				pod.Name,
				ctx.E2ENamespace,
				key,
				value)
		}
	}

	return errors.New("e2e runner pod not found")
}

func labelPod(client k8s.Client, name, namespace, key, value string) error {
	pod := corev1.Pod{}
	if err := client.Get(context.Background(), types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, &pod); err != nil {
		return err
	}

	pod.Labels[key] = value
	return client.Update(context.Background(), &pod)
}

// IsGKE returns if the current Kubernetes cluster is a GKE cluster based on the Kubernetes version of the current test context.
func IsGKE(v version.Version) bool {
	// cloud providers append the name of the k8s platform in the version prefix (e.g.: 1.21.6-gke.1503)
	return strings.Contains(v.String(), "gke")
}

func SkipUntilResolution(t *testing.T, knownIssueNumber int) {
	t.Helper()
	t.Skipf("Skip until we understand why it is failing, see https://github.com/elastic/cloud-on-k8s/issues/%d", knownIssueNumber)
}

// This simulates "kubectl delete elastic" in the e2e namespace.
func deleteTestResources(ctx context.Context) error {
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "while getting kubernetes config")
		return err
	}
	clntset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "while getting clientset")
		return err
	}

	groupVersionToResourceListMap := map[string][]v1.APIResource{}
	_, resources, err := clntset.Discovery().ServerGroupsAndResources()
	if err != nil {
		log.Error(err, "while running kubernetes client discovery")
		return err
	}
	for _, resource := range resources {
		if strings.Contains(resource.GroupVersion, "k8s.elastic.co") {
			groupVersionToResourceListMap[resource.GroupVersion] = resource.APIResources
		}
	}

	for _, namespace := range Ctx().Operator.ManagedNamespaces {
		dynamicClient := dynamic.New(clntset.RESTClient())
		for gv, resources := range groupVersionToResourceListMap {
			gvSlice := strings.Split(gv, "/")
			if len(gvSlice) != 2 {
				continue
			}
			group, version := gvSlice[0], gvSlice[1]
			for _, resource := range resources {
				if err := dynamicClient.Resource(schema.GroupVersionResource{
					Group:    group,
					Resource: resource.Name,
					Version:  version,
				}).Namespace(namespace).DeleteCollection(ctx, v1.DeleteOptions{}, v1.ListOptions{}); err != nil && !api_errors.IsNotFound(err) {
					msg := fmt.Sprintf("while deleting elastic resources in %s", namespace)
					log.Error(err, msg, "group", group, "resource", resource.Name, "version", version)
					return err
				}
			}
		}

		list, err := clntset.CoreV1().Secrets(namespace).List(ctx, v1.ListOptions{})
		if err != nil {
			return fmt.Errorf("while listing all secrets in namespace %s: %w", namespace, err)
		}
		for _, secret := range list.Items {

			secretName := secret.GetName()
			if strings.HasPrefix(secretName, "sh.helm.release") {
				helmReleaseTokes := strings.Split(secretName, ".")
				if len(helmReleaseTokes) == 6 {
					settings := cli.New()
					settings.SetNamespace(namespace)
					actionConfig := &action.Configuration{}
					if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), "",
						func(format string, v ...interface{}) {}); err != nil {
						return err
					}

					uninstallAction := action.NewUninstall(actionConfig)
					if _, err := uninstallAction.Run(helmReleaseTokes[4]); err != nil {
						return err
					}
					continue
				}
			}

			if err := clntset.CoreV1().Secrets(namespace).Delete(ctx, secretName, v1.DeleteOptions{}); err != nil {
				return fmt.Errorf("while deleting secret %s in namespace %s: %w", secretName, namespace, err)
			}
		}

		pods, err := clntset.CoreV1().Pods(namespace).List(ctx, v1.ListOptions{})
		if err != nil {
			return fmt.Errorf("while listing all pods in namespace %s: %w", namespace, err)
		}
		for _, pod := range pods.Items {
			if err := clntset.CoreV1().Pods(namespace).Delete(ctx, pod.GetName(), v1.DeleteOptions{}); err != nil {
				return fmt.Errorf("while deleting pod %s in namespace %s: %w", pod.GetName(), namespace, err)
			}
		}
	}
	return nil
}
