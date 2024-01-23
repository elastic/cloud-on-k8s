// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/command"
)

// canRunDiagnostics will determine if this e2e test run has the ability to run eck-diagnostics after
// each test failure, which includes uploading the resulting zip file to a GS bucket. If the job name
// is set (not empty), the google credentials file exists, and the eck-diagnostics binary exists
// then we should be able to run diagnostics.
func canRunDiagnostics() bool {
	// If we're not in Kubernetes, then don't run diagnostics on e2e test failures.
	if _, inK8s := os.LookupEnv("KUBERNETES_SERVICE_HOST"); !inK8s {
		return false
	}
	if _, err := exec.LookPath("eck-diagnostics"); err != nil {
		return false
	}
	return true
}

func run(ctx context.Context, executable string, args ...string) {
	// If /tmp is not set as HOME, the k8s client can't cache responses,
	// and a single run of diagnostics is incredibly slow.
	_, _, err := command.New(executable, args...).WithEnv("HOME=/tmp").Build().Execute(ctx)
	if err != nil {
		log.Error(err, "while running", "cmd", executable)
	}
}

func maybeRunECKDiagnostics(ctx context.Context, testName string, step Step) {
	testCtx := Ctx()
	if !canRunDiagnostics() {
		return
	}
	log.Info("Running eck-diagnostics", "cluster", testCtx.ClusterName, "test", testName, "step", step.Name)

	otherNS := append([]string{testCtx.E2ENamespace}, testCtx.Operator.ManagedNamespaces...)
	// The following appends the clustername, test name, and it's sub-test names together with a '-'.
	// The cluster name is added to the eck-diagnostics file name to avoid conflicts at the last step
	// of the e2e tests where all diagnostics are downloaded locally to the same directory, and uploaded to buildkite as artifacts.
	// Example: For TestAutoscalingLegacy/Secrets_should_eventually_be_created in cluster eck-bk-e2e-pr-2373:
	// testName: TestAutoscalingLegacy, step.Name: Secrets_should_eventually_be_created, clustername: eck-bk-e2e-pr-2373
	fullTestName := fmt.Sprintf("%s-%s-%s", testCtx.ClusterName, testName, step.Name)
	// Convert any spaces to "_", and "/" to "-" in the test name.
	normalizedTestName := strings.ReplaceAll(strings.ReplaceAll(fullTestName, " ", "_"), "/", "-")

	run(ctx, "eck-diagnostics",
		"--output-directory", Ctx().ArtefactsDir,
		"-n", fmt.Sprintf("eck-diagnostics-%s.zip", normalizedTestName),
		"-o", testCtx.Operator.Namespace,
		"-r", strings.Join(otherNS, ","),
		"--run-agent-diagnostics")
}
