// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import "time"

const (
	// DefaultCertValidity makes new certificates default to a 1 year expiration
	DefaultCertValidity = 365 * 24 * time.Hour
	// DefaultRotateBefore defines how long before expiration a certificate
	// should be re-issued
	DefaultRotateBefore = 24 * time.Hour
)

// RotationParams defines validity and a safety margin for certificate rotation.
type RotationParams struct {
	// Validity is the validity duration of a newly created cert.
	Validity time.Duration
	// RotateBefore defines how long before expiration certificates should be rotated.
	RotateBefore time.Duration
}

// ShouldRotateIn computes the duration after which a certificate rotation should be scheduled
// in order for the CA cert to be rotated before it expires.
func ShouldRotateIn(now time.Time, certExpiration time.Time, caCertRotateBefore time.Duration) time.Duration {
	// make sure we are past the safety margin when rotating, by making it a little bit shorter
	safetyMargin := caCertRotateBefore - 1*time.Second
	requeueTime := certExpiration.Add(-safetyMargin)
	requeueIn := requeueTime.Sub(now)
	if requeueIn < 0 {
		// requeue asap
		requeueIn = 0
	}
	return requeueIn
}
