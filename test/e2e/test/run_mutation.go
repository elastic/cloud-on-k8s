// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/labels"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	// testes "github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

var (
	printed bool
)

// RunMutations tests resources changes on given resources.
// If the resource to mutate to is the same as the original resource, then all tests should still pass.
// //nolint:thelper
func RunMutations(t *testing.T, creationBuilders []Builder, mutationBuilders []Builder) {
	skipIfIncompatibleBuilders(t, append(creationBuilders, mutationBuilders...)...)
	k := NewK8sClientOrFatal()
	steps := StepList{}

	for _, toCreate := range creationBuilders {
		steps = steps.WithSteps(toCreate.InitTestSteps(k))
	}
	for _, toCreate := range creationBuilders {
		steps = steps.WithSteps(toCreate.CreationTestSteps(k))
	}
	steps = steps.WithSteps(StepList{
		{
			Name: "get cgroup information from elasticsearch",
			Test: Eventually(func() error {
				ctx := Ctx()
				namespace := fmt.Sprintf("%s-%s", ctx.TestRun, "mercury")
				listOptions := k8sclient.ListOptions{
					Namespace: namespace,
					LabelSelector: labels.SelectorFromSet(labels.Set{
						commonv1.TypeLabelName: label.Type,
					}),
				}
				pods, err := k.GetPods(&listOptions)
				if err != nil {
					return err
				}
				if len(pods) == 0 {
					return errors.New("no pods found")
				}

				// exec into the pod to list keystore entries
				stdout, _, err := k.Exec(k8s.ExtractNamespacedName(&pods[0]),
					[]string{"cat", "/proc/1/cgroup"})
				if err != nil {
					return err
				}
				var cpuAcctData string
				if !printed {
					fmt.Printf("cgroup data: %s", stdout)
				}
				printed = true
				for _, line := range strings.Split(stdout, "\n") {
					for _, controlGroup := range strings.Split(line, ":") {
						if strings.Contains(controlGroup, "cpuacct") {
							cpuAcctData = strings.Split(line, ":")[2]
						}
					}
				}
				fullPath := path.Join("/sys/fs/cgroup/cpu,cpuacct", cpuAcctData, "cpuacct.usage")
				fmt.Printf("cpuacct data full path: %s", fullPath)
				if _, err := os.Stat(fullPath); err != nil {
					return fmt.Errorf("while attempting to stat %s: %w", fullPath, err)
				}

				fmt.Printf("cpuacct.usage file exists")

				return nil
			}),
		}})
	for _, toCreate := range creationBuilders {
		steps = steps.WithSteps(CheckTestSteps(toCreate, k))
	}

	// Trigger some mutations
	for _, mutateTo := range mutationBuilders {
		steps = steps.WithSteps(mutateTo.MutationTestSteps(k))
	}

	// Delete using the original builder (so that we can use it as a mutation builder as well)
	for _, toCreate := range creationBuilders {
		steps = steps.WithSteps(toCreate.DeletionTestSteps(k))
	}

	steps.RunSequential(t)
}

//nolint:thelper
func RunMutationsWhileWatching(t *testing.T, creationBuilders []Builder, mutationBuilders []Builder, watchers []Watcher) {
	skipIfIncompatibleBuilders(t, append(creationBuilders, mutationBuilders...)...)
	k := NewK8sClientOrFatal()
	steps := StepList{}

	for _, toCreate := range creationBuilders {
		steps = steps.WithSteps(toCreate.InitTestSteps(k))
	}
	for _, toCreate := range creationBuilders {
		steps = steps.WithSteps(toCreate.CreationTestSteps(k))
	}
	for _, toCreate := range creationBuilders {
		steps = steps.WithSteps(CheckTestSteps(toCreate, k))
	}

	// Start watchers
	for _, watcher := range watchers {
		steps = steps.WithStep(watcher.StartStep(k))
	}

	// Trigger some mutations
	for _, mutateTo := range mutationBuilders {
		steps = steps.WithSteps(mutateTo.MutationTestSteps(k))
	}

	for _, watcher := range watchers {
		steps = steps.WithStep(watcher.StopStep(k))
	}

	// Delete using the original builder (so that we can use it as a mutation builder as well)
	for _, toCreate := range creationBuilders {
		steps = steps.WithSteps(toCreate.DeletionTestSteps(k))
	}

	steps.RunSequential(t)
}

// RunMutations tests one resource change on a given resource.
//
//nolint:thelper
func RunMutation(t *testing.T, toCreate Builder, mutateTo Builder) {
	RunMutations(t, []Builder{toCreate}, []Builder{mutateTo})
}
