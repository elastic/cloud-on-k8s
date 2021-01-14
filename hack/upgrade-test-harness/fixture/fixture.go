// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package fixture

import (
	"errors"
	"fmt"
	"time"

	"github.com/elastic/cloud-on-k8s/hack/upgrade-test-harness/k8s"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/util/retry"
)

const (
	deleteTimeout   = 120 * time.Second
	retryMultiplier = 0
	retryJitter     = 0
)

var ErrRetry = errors.New("retriable error")

// TestContext holds the context for the current test.
type TestContext struct {
	*k8s.Kubectl
	*Janitor
	*zap.SugaredLogger
	backoff wait.Backoff
}

// NewTestContext creates a new test context.
func NewTestContext(confFlags *genericclioptions.ConfigFlags, retryCount int, retryDelay, retryTimeout time.Duration) (*TestContext, error) {
	kubectl, err := k8s.NewKubectl(confFlags)
	if err != nil {
		return nil, fmt.Errorf("failed to create K8S helper: %w", err)
	}

	backoff := wait.Backoff{
		Duration: retryDelay,
		Factor:   retryMultiplier,
		Jitter:   retryJitter,
		Steps:    retryCount,
		Cap:      retryTimeout,
	}

	return &TestContext{
		Kubectl:       kubectl,
		Janitor:       &Janitor{},
		SugaredLogger: zap.S().Named("fixture"),
		backoff:       backoff,
	}, nil
}

// WithName creates a copy of the context with a new name.
func (tc *TestContext) WithName(name string) *TestContext {
	return &TestContext{
		Kubectl:       tc.Kubectl,
		Janitor:       tc.Janitor,
		SugaredLogger: tc.With("step", name),
		backoff:       tc.backoff,
	}
}

// Fixture is a container for a group of related tests.
type Fixture struct {
	Name  string
	Steps []*TestStep
}

// Execute the fixture.
func (f *Fixture) Execute(ctx *TestContext) error {
	ctx = ctx.WithName(f.Name)

	for _, step := range f.Steps {
		if err := step.Execute(ctx.WithName(step.Name)); err != nil {
			return err
		}
	}

	return nil
}

// TestStep describes a step in the test.
type TestStep struct {
	Name      string
	Action    func(*TestContext) error
	Retriable func(error) bool
}

// Execute executes the current test step.
func (ts *TestStep) Execute(ctx *TestContext) error {
	ctx.Infof("[START] %s", ts.Name)

	err := retry.OnError(ctx.backoff, ts.Retriable, func() error { return ts.Action(ctx) })
	if err != nil {
		ctx.Errorw(fmt.Sprintf("[FAIL] %s", ts.Name), "error", err)
	} else {
		ctx.Infof("[DONE] %s", ts.Name)
	}

	return err
}

// retryRetriable is a convenience function to create a test step that is retried if the error is deemed retriable.
func retryRetriable(name string, action func(*TestContext) error) *TestStep {
	return &TestStep{
		Name:   name,
		Action: action,
		Retriable: func(err error) bool {
			return errors.Is(err, ErrRetry) || apierrors.IsNotFound(err) || apierrors.IsConflict(err)
		},
	}
}

// noRetry is a convenience function to create a test step that is not retried.
func noRetry(name string, action func(*TestContext) error) *TestStep {
	return &TestStep{
		Name:      name,
		Action:    action,
		Retriable: func(_ error) bool { return false },
	}
}

// pause is a convenience test step for pausing for the given duration.
func pause(duration time.Duration) *TestStep {
	return &TestStep{
		Name: fmt.Sprintf("Pause[%s]", duration),
		Action: func(_ *TestContext) error {
			time.Sleep(duration)
			return nil
		},
		Retriable: func(_ error) bool { return false },
	}
}

// applyManifests is a convenience test function that applies the provided manifests to the cluster.
func applyManifests(path string) func(*TestContext) error {
	return func(ctx *TestContext) error {
		manifests, err := ctx.LoadResources(path)
		if err != nil {
			return err
		}

		ctx.AddCleanupFunc(deleteResources(manifests))

		return ctx.CreateOrUpdate(manifests)
	}
}

// deleteManifests is a convenience test function that deletes the provided manifests to the cluster.
func deleteManifests(path string) func(*TestContext) error {
	return func(ctx *TestContext) error {
		manifests, err := ctx.LoadResources(path)
		if err != nil {
			return err
		}

		return ctx.Delete(manifests, deleteTimeout)
	}
}

// CleanupFunc is a function for cleaning up resources after the test run.
type CleanupFunc func(*TestContext) error

// Janitor keeps track of cleanup tasks and performs them at the end of the test run.
type Janitor struct {
	backlog []CleanupFunc
}

// AddCleanupFunc adds a new cleanup task to the stack.
func (j *Janitor) AddCleanupFunc(cf CleanupFunc) {
	j.backlog = append(j.backlog, cf)
}

// Cleanup performs the cleanup.
func (j *Janitor) Cleanup(ctx *TestContext) error {
	var errors error

	for i := len(j.backlog) - 1; i >= 0; i-- {
		if err := j.backlog[i](ctx); err != nil {
			errors = multierror.Append(errors, err)
		}
	}

	return errors
}

// deleteResources is a convenience function for creating a cleanup function that deletes a set of resources.
func deleteResources(resources *resource.Result) CleanupFunc {
	return func(ctx *TestContext) error {
		return ctx.Delete(resources, deleteTimeout)
	}
}
