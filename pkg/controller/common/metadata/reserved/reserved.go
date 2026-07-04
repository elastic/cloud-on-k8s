// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package reserved identifies metadata keys owned by ECK under the *.k8s.elastic.co label
// namespace. It is shared by volume claim template label validation and metadata propagation.
package reserved

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// RootSubdomain is the shortest DNS subdomain (k8s.elastic.co) reserved for ECK-managed keys.
const RootSubdomain = "k8s.elastic.co"

// IsReservedLabelKey reports whether key is a Kubernetes label key owned by ECK and must not
// be set or copied by users. Keys use the recommended label form domain/name where domain
// is k8s.elastic.co, ends with .k8s.elastic.co (e.g. elasticsearch.k8s.elastic.co), or
// contains an extra label under k8s (e.g. eck.k8s.alpha.elastic.co).
func IsReservedLabelKey(key string) bool {
	domain, _, ok := strings.Cut(key, "/")
	if !ok {
		return false
	}
	return isReservedECKDomain(domain)
}

// IsReservedAnnotationKey reports whether an annotation key must not be propagated from a
// parent resource to children. It applies the same ECK domain rules as label keys, plus
// kubectl's last-applied-configuration annotation.
func IsReservedAnnotationKey(key string) bool {
	if key == corev1.LastAppliedConfigAnnotation {
		return true
	}
	return IsReservedLabelKey(key)
}

func isReservedECKDomain(domain string) bool {
	if domain == RootSubdomain {
		return true
	}
	if strings.HasSuffix(domain, "."+RootSubdomain) {
		return true
	}
	// eck.k8s.alpha.elastic.co and similar: still under the ECK k8s.elastic.co tree.
	if strings.Contains(domain, ".k8s.") && strings.HasSuffix(domain, ".elastic.co") {
		return true
	}
	return false
}
