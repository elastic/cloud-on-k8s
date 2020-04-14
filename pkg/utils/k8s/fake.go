// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package k8s

import (
	controllerscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"k8s.io/apimachinery/pkg/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Scheme() *runtime.Scheme {
	controllerscheme.SetupScheme()
	return k8sscheme.Scheme
}

func FakeClient(initObjs ...runtime.Object) client.Client {
	return fake.NewFakeClientWithScheme(Scheme(), initObjs...)
}

func WrappedFakeClient(initObjs ...runtime.Object) Client {
	return WrapClient(FakeClient(initObjs...))
}
