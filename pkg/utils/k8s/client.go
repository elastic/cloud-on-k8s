// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package k8s

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	scheme.SetupScheme()
}

func Scheme() *runtime.Scheme {
	return clientgoscheme.Scheme
}

type Client = client.Client
