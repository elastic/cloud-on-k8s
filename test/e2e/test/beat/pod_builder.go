// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package beat

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

// Builder to create a Pod. It can be used as a source of logging/metric data for Beat (deployed separately) to collect.
type PodBuilder struct {
	Pod    corev1.Pod
	Logged string
}

func (pb PodBuilder) SkipTest() bool {
	return false
}

func NewPodBuilder(name string) PodBuilder {
	return newPodBuilder(name, rand.String(4))
}

func newPodBuilder(name, suffix string) PodBuilder {
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: test.Ctx().ManagedNamespace(0),
		Labels:    map[string]string{run.TestNameLabel: name},
	}

	// inject random string into the logs to allow validating whether they end up in ES easily
	loggedString := fmt.Sprintf("_%s_", rand.String(6))

	uid1001 := int64(1001)
	return PodBuilder{
		Pod: corev1.Pod{
			ObjectMeta: meta,
			Spec: corev1.PodSpec{
				AutomountServiceAccountToken: ptr.To[bool](false),
				Containers: []corev1.Container{
					{
						Name:  "ubuntu",
						Image: "busybox",
						Command: []string{
							"sh",
							"-c",
							fmt.Sprintf("while [ true ]; do echo \"$(date) - %s\"; sleep 5; done", loggedString),
						},
					},
				},
				TerminationGracePeriodSeconds: ptr.To[int64](0),
				SecurityContext: &corev1.PodSecurityContext{
					// Security policies forbid root user on secured clusters
					RunAsUser: &uid1001,
				},
			},
		},
		Logged: loggedString,
	}.
		WithSuffix(suffix).
		WithLabel(run.TestNameLabel, name)
}

func (pb PodBuilder) WithSuffix(suffix string) PodBuilder {
	if suffix != "" {
		pb.Pod.ObjectMeta.Name = pb.Pod.ObjectMeta.Name + "-" + suffix
	}
	return pb
}

func (pb PodBuilder) WithLabel(key, value string) PodBuilder {
	if pb.Pod.Labels == nil {
		pb.Pod.Labels = make(map[string]string)
	}
	pb.Pod.Labels[key] = value

	return pb
}

func (pb PodBuilder) RuntimeObjects() []client.Object {
	return []client.Object{&pb.Pod}
}

func (pb PodBuilder) InitTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "K8S should be accessible",
			Test: test.Eventually(func() error {
				pods := corev1.PodList{}
				return k.Client.List(context.Background(), &pods)
			}),
		},
		{
			Name: "Label test pods",
			Test: test.Eventually(func() error {
				return test.LabelTestPods(
					k.Client,
					test.Ctx(),
					run.TestNameLabel,
					pb.Pod.Labels[run.TestNameLabel])
			}),
			Skip: func() bool {
				return test.Ctx().Local
			},
		},
		{
			Name: "Remove pod if it already exists",
			Test: test.Eventually(func() error {
				for _, obj := range pb.RuntimeObjects() {
					err := k.Client.Delete(context.Background(), obj)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
				}
				// wait for pod to disappear
				var pod corev1.Pod
				err := k.Client.Get(context.Background(), types.NamespacedName{
					Namespace: pb.Pod.Namespace,
					Name:      pb.Pod.Name,
				}, &pod)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				if err == nil {
					return fmt.Errorf("pod %s is still there", k8s.ExtractNamespacedName(&pb.Pod))
				}
				return nil
			}),
		},
	}
}

func (pb PodBuilder) CreationTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{}.
		WithSteps(test.StepList{
			test.Step{
				Name: "Creating a Pod should succeed",
				Test: test.Eventually(func() error {
					return k.CreateOrUpdate(pb.RuntimeObjects()...)
				}),
			},
			test.Step{
				Name: "Pod should be created",
				Test: test.Eventually(func() error {
					var createdPod corev1.Pod
					return k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&pb.Pod), &createdPod)
				}),
			},
		})
}

func (pb PodBuilder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		test.Step{
			Name: "Pod should be ready and running",
			Test: test.Eventually(func() error {
				var pod corev1.Pod
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&pb.Pod), &pod); err != nil {
					return err
				}

				// pod is running
				if pod.Status.Phase != corev1.PodRunning {
					return fmt.Errorf("pod not running yet")
				}

				// pod is ready
				if !k8s.IsPodReady(pod) {
					return fmt.Errorf("pod not ready yet")
				}

				return nil
			}),
		},
	}
}

func (pb PodBuilder) CheckStackTestSteps(*test.K8sClient) test.StepList {
	return test.StepList{} // nothing to do
}

func (pb PodBuilder) UpgradeTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Applying pod mutation should succeed",
			Test: test.Eventually(func() error {
				var pod corev1.Pod
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&pb.Pod), &pod); err != nil {
					return err
				}
				pod.Spec = pb.Pod.Spec
				return k.Client.Update(context.Background(), &pod)
			}),
		}}
}

func (pb PodBuilder) DeletionTestSteps(k *test.K8sClient) test.StepList {
	return []test.Step{
		{
			Name: "Deleting the resources should return no error",
			Test: test.Eventually(func() error {
				for _, obj := range pb.RuntimeObjects() {
					err := k.Client.Delete(context.Background(), obj)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
				}
				return nil
			}),
		},
		{
			Name: "The resources should not be there anymore",
			Test: test.Eventually(func() error {
				for _, obj := range pb.RuntimeObjects() {
					objCopy := k8s.DeepCopyObject(obj)
					err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(obj), objCopy)
					if err != nil {
						if apierrors.IsNotFound(err) {
							continue
						}
					}
					return errors.Wrap(err, "expected 404 not found API error here")
				}
				return nil
			}),
		},
		{
			Name: "Pod should be eventually be removed",
			Test: test.Eventually(func() error {
				// wait for pod to disappear
				var pod corev1.Pod
				err := k.Client.Get(context.Background(), types.NamespacedName{
					Namespace: pb.Pod.Namespace,
					Name:      pb.Pod.Name,
				}, &pod)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				if err == nil {
					return fmt.Errorf("pod %s is still there", k8s.ExtractNamespacedName(&pb.Pod))
				}
				return nil
			}),
		},
	}
}

func (pb PodBuilder) MutationTestSteps(_ *test.K8sClient) test.StepList {
	panic("implement me")
}

var _ test.Builder = Builder{}
