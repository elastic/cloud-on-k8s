// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package fixture

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
)

// TestRemoveFinalizers removes the finalizers from alpha objects.
func TestRemoveFinalizers(param TestParam) *Fixture {
	return &Fixture{
		Name: "TestRemoveFinalizers",
		Steps: []*TestStep{
			noRetry("RemoveESFinalizers", removeFinalizers("elasticsearch", esName, param.GVR("elasticsearch"))),
			noRetry("RemoveKBFinalizers", removeFinalizers("kibana", kbName, param.GVR("kibana"))),
			noRetry("RemoveAPMFinalizers", removeFinalizers("apmserver", apmName, param.GVR("apmserver"))),
		},
	}
}

func removeFinalizers(kind, name string, gvr schema.GroupVersionResource) func(*TestContext) error {
	return func(ctx *TestContext) error {
		resources := ctx.GetResources(ctx.Namespace(), kind, name)

		dynamicClient, err := ctx.DynamicClient()
		if err != nil {
			return err
		}

		return resources.Visit(func(info *resource.Info, err error) error {
			if err != nil {
				return err
			}

			runtimeObj, err := resources.Object()
			if err != nil {
				return err
			}

			obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(runtimeObj)
			if err != nil {
				return err
			}

			u := &unstructured.Unstructured{Object: obj}
			u.SetFinalizers(nil)

			_, err = dynamicClient.Resource(gvr).Namespace(ctx.Namespace()).Update(context.TODO(), u, metav1.UpdateOptions{})

			return err
		})
	}
}
