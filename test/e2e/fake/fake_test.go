// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build fake || e2e

package fake

import (
	"fmt"
	"testing"
	"time"
)

func TestFailingStuff(t *testing.T) {
	time.Sleep(10 * time.Second)
	t.Fail()
}

func TestSucceedingStuff(t *testing.T) {
	time.Sleep(10 * time.Second)
	t.Log(fmt.Sprintf("success"))
}
