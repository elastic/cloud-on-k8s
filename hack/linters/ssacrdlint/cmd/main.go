// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// ssacrdlint is a standalone go-vet-style binary wrapping the ssacrdlint
// analyzer. Run it with:
//
//	go vet -vettool=$(which ssacrdlint) -tags release ./pkg/... ./cmd/...
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/elastic/cloud-on-k8s/hack/linters/ssacrdlint"
)

func main() {
	singlechecker.Main(ssacrdlint.Analyzer)
}
