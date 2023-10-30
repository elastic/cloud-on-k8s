// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Step represents a single test
type Step struct {
	Name      string
	Test      func(t *testing.T)
	Skip      func() bool // returns true if the test should be skipped
	OnFailure func()      // additional step specific callback on failure
}

// StepList defines a list of Step
type StepList []Step

// WithSteps appends the given StepList to the StepList
func (l StepList) WithSteps(testSteps StepList) StepList {
	return append(l, testSteps...)
}

// WithStep appends the given Step to the StepList
func (l StepList) WithStep(testStep Step) StepList {
	return append(l, testStep)
}

// RunSequential runs the StepList sequentially, continuing on any errors.
// If the tests are running within CI, the following occurs:
//   - ECK-diagnostics is run after each failure
//   - The resulting Zip file is uploaded to a GS Bucket
//   - All Zip files are downloaded to local agent when tests complete and are
//     added as Buildkite artifacts.
//
//nolint:thelper
func (l StepList) RunSequential(t *testing.T) {
	for _, ts := range l {
		if ts.Skip != nil && ts.Skip() {
			log.Info("Skipping test", "name", ts.Name)
			continue
		}
		if !t.Run(ts.Name, ts.Test) {
			logf.Log.Error(fmt.Errorf("test %s failed", ts.Name), "continuing with additional tests")
			if ts.OnFailure != nil {
				ts.OnFailure()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()

			maybeRunECKDiagnostics(ctx, t.Name(), ts)
			if err := deleteTestResources(ctx); err != nil {
				log.Error(err, "while deleting elastic resources")
			}
			break // we don't want to continue with this particular test
		}
	}
}

type StepsFunc func(k *K8sClient) StepList

func EmptySteps(_ *K8sClient) StepList {
	return StepList{}
}
