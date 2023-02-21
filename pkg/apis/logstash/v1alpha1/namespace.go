// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

func GetOrDefaultNamespace(ns string) string {
	if ns != "" {
		return ns
	}
	return "default"
}
