// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package admission

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// Warnings represents warning messages.
type Warnings []string

// Validator defines functions for validating an operation.
// The custom resource kind which implements this interface can validate itself.
// To validate the custom resource with another specific struct, use CustomValidator instead.
//
// Deprecated: Consider CustomValidator instead, this only restores the old ctrl-runtime interface for bwc.
type Validator interface {
	runtime.Object

	// ValidateCreate validates the object on creation.
	// The optional warnings will be added to the response as warning messages.
	// Return an error if the object is invalid.
	ValidateCreate() (warnings Warnings, err error)

	// ValidateUpdate validates the object on update. The oldObj is the object before the update.
	// The optional warnings will be added to the response as warning messages.
	// Return an error if the object is invalid.
	ValidateUpdate(old runtime.Object) (warnings Warnings, err error)

	// ValidateDelete validates the object on deletion.
	// The optional warnings will be added to the response as warning messages.
	// Return an error if the object is invalid.
	ValidateDelete() (warnings Warnings, err error)
}
