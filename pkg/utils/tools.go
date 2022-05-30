// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build tools

package utils

// dummy import to maintain the version of controller-gen in go.mod
import (
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
)
