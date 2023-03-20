// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/command"
)

var once sync.Once

// canRunDiagnostics will determine if this e2e test run has the ability to run eck-diagnostics after
// each test failure, which includes uploading the resulting zip file to a GS bucket. If the job name
// is set (not empty) and the google credentials file exists, then we should be able to run diagnostics.
func canRunDiagnostics(ctx Context) bool {
	if _, err := os.Stat(ctx.GCPCredentialsPath); err != nil && errors.Is(err, fs.ErrNotExist) {
		return false
	} else if err != nil {
		log.Error(err, "while checking for existence of %s", ctx.GCPCredentialsPath)
		return false
	}
	if ctx.JobName == "" {
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
	if !canRunDiagnostics(testCtx) {
		return
	}
	log.Info("running eck-diagnostics job")
	once.Do(func() {
		run(ctx, "initialising gs-util", "gcloud", "auth", "activate-service-account", fmt.Sprintf("--key-file=%s", testCtx.GCPCredentialsPath))
	})
	otherNS := append([]string{testCtx.E2ENamespace}, testCtx.Operator.ManagedNamespaces...)
	// The following appends the test, and it's sub-test names together with a '-'.
	// Example: For TestAutoscalingLegacy/Secrets_should_eventually_be_created:
	// testName: TestAutoscalingLegacy, step.Name: Secrets_should_eventually_be_created
	fullTestName := fmt.Sprintf("%s-%s", testName, step.Name)
	// Convert any spaces to "_", and "/" to "-" in the test name.
	normalizedTestName := strings.ReplaceAll(strings.ReplaceAll(fullTestName, " ", "_"), "/", "-")
	run(ctx, "eck-diagnostics", "--output-directory", "/tmp", "-n", fmt.Sprintf("eck-diagnostic-%s.zip", normalizedTestName), "-o", testCtx.Operator.Namespace, "-r", strings.Join(otherNS, ","), "--run-agent-diagnostics")
	run(ctx, "gsutil", "cp", "/tmp/*.zip", fmt.Sprintf("gs://%s/jobs/%s/", testCtx.GSBucketName, testCtx.ClusterName))
}
