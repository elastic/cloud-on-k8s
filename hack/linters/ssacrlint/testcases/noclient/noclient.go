// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package noclient is an ssacrlint test fixture that does not import
// sigs.k8s.io/controller-runtime/pkg/client. The analyzer must exit early
// and emit no diagnostics for such packages.
package noclient

// Update is a function named Update that is unrelated to controller-runtime's
// client.Writer; it must not be flagged.
func Update() {}
