// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"encoding/json"
	"fmt"
	"log"

	eckscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/pkg/resource"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func main() {
	licensingInfo, err := resource.NewLicensingReporter(newK8sClient()).Get()
	if err != nil {
		log.Fatal(err, "Failed to get licensing info")
	}

	bytes, err := json.Marshal(licensingInfo)
	if err != nil {
		log.Fatal(err, "Failed to marshal licensing info")
	}

	fmt.Print(string(bytes))
}

func newK8sClient() client.Client {
	cfg, err := config.GetConfig()
	if err != nil {
		log.Fatal(err, "Failed to get a Kubernetes config")
	}

	err = eckscheme.SetupScheme()
	if err != nil {
		log.Fatal(err, "Failed to set up the ECK scheme")
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		log.Fatal(err, "Failed to create a new Kubernetes client")
	}

	return c
}
