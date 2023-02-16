// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build logstash || e2e

package logstash

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/logstash"
)

func TestSingleLogstash(t *testing.T) {
	name := "test-single-logstash"
	logstashBuilder := logstash.NewBuilder(name).
		WithNodeCount(1)
	test.Sequence(nil, test.EmptySteps, logstashBuilder).RunSequential(t)
}

func TestMultipleLogstashes(t *testing.T) {
	name := "test-multiple-logstashes"
	logstashBuilder := logstash.NewBuilder(name).
		WithNodeCount(3)
	test.Sequence(nil, test.EmptySteps, logstashBuilder).RunSequential(t)
}

func TestLogstashServerVersionUpgradeToLatest8x(t *testing.T) {
	srcVersion, dstVersion := test.GetUpgradePathTo8x(test.Ctx().ElasticStackVersion)

	name := "test-ls-version-upgrade-8x"

	logstash := logstash.NewBuilder(name).
		WithNodeCount(2).
		WithVersion(srcVersion).
		WithRestrictedSecurityContext()

	logstashUpgraded := logstash.WithVersion(dstVersion).WithMutatedFrom(&logstash)

	test.RunMutations(t, []test.Builder{logstash}, []test.Builder{logstashUpgraded})
}
