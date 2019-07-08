// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"flag"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	defaultElasticStackVersion = "7.1.1"
	defaultNamespace           = "e2e"
)

var (
	log = logf.Log.WithName("e2e-params")

	ElasticStackVersion string
	Namespace           string
	AutoPortForward     bool
	TestLicense         string
)

func init() {
	flag.StringVar(&ElasticStackVersion, "version", defaultElasticStackVersion, "Elastic Stack version")
	flag.StringVar(&Namespace, "namespace", defaultNamespace, "Namespace")
	flag.StringVar(&TestLicense, "test-license", "", "path to Enterprise license to be used for testing")
	flag.BoolVar(&AutoPortForward, "auto-port-forward", false, "enables automatic port-forwarding "+
		"(for dev use only as it exposes k8s resources on ephemeral ports to localhost)")
	flag.Parse()

	logf.SetLogger(logf.ZapLogger(true))
	log.Info("Info", "version", ElasticStackVersion, "namespace", Namespace)
}
