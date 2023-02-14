// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	controllerscheme "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v2/pkg/license"
)

// Simple program that returns the licensing information, including the total memory of all Elastic managed components by
// the operator and its equivalent in "Enterprise Resource Units".
//
// The main objective of its existence is to show a use of the ResourceReporter and also to propose an alternative to
// immediately retrieve the licensing information.
//
// Example of use:
//
//  > go run cmd/licensing-info/main.go -operator-namespace <operator-namespace>
//  {
//    "timestamp": "2019-12-17T11:56:02+01:00",
//    "license_level": "basic",
//    "memory": "5.37GB",
//    "enterprise_resource_units": "1"
//  }
//

func main() {
	var operatorNamespace string
	flag.StringVar(&operatorNamespace, "operator-namespace", "elastic-system", "indicates the namespace where the operator is deployed")
	flag.Parse()
	licensingInfo, err := license.NewResourceReporter(newK8sClient(), operatorNamespace, nil).Get(context.Background())
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

	controllerscheme.SetupScheme()

	c, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		log.Fatal(err, "Failed to create a new Kubernetes client")
	}

	return c
}
