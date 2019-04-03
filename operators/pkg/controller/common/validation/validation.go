/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package validation

// Result contains validation results.
type Result struct {
	Error   error
	Allowed bool
	Reason  string
}

// OK is a successfull validation result.
var OK = Result{Allowed: true}
