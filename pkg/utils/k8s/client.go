// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package k8s

import (
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/scheme"
)

func init() {
	scheme.SetupScheme()
}

func Scheme() *runtime.Scheme {
	return clientgoscheme.Scheme
}

type Client = client.Client
