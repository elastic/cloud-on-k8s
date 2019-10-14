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
	// MaxReconciliationPeriod defines the maximum period of time between 2 certificates rotation.
	MaxReconciliationPeriod = 10 * time.Hour
)

// RotationParams defines validity and a safety margin for certificate rotation.
type RotationParams struct {
	// Validity is the validity duration of a newly created cert.
	Validity time.Duration
	// RotateBefore defines how long before expiration certificates should be rotated.
	RotateBefore time.Duration
}

// shouldRotateIn computes the duration after which a certificate rotation should be scheduled
// in order for the CA cert to be rotated before it expires.
func shouldRotateIn(now time.Time, certExpiration time.Time, caCertRotateBefore time.Duration) time.Duration {
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

// ShouldReconcileIn returns the duration after which a reconciliation should be done
// to make sure certificates do not expire.
func ShouldReconcileIn(now time.Time, certExpiration time.Time, caCertRotateBefore time.Duration) time.Duration {
	rotateIn := shouldRotateIn(now, certExpiration, caCertRotateBefore)
	if rotateIn > MaxReconciliationPeriod {
		// We don't want to wait for rotateIn to be reached, because of an underlying leaky timer issue.
		// See https://github.com/elastic/cloud-on-k8s/issues/1984.
		// TODO: remove once https://github.com/kubernetes/client-go/issues/701 is fixed.
		return MaxReconciliationPeriod
	}
	return rotateIn
}
