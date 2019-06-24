// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package name

import (
	"fmt"
	"strings"

	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("name")
)

const (
	// MaxNameLength is the maximum length of a resource name.
	MaxNameLength = 63
)

// Namer assists with constructing names for K8s resources and avoiding collisions by ensuring that certain suffixes
// are always used, and prevents the use of too long suffixes.
type Namer struct {
	// MaxSuffixLength is the maximum allowable length of a suffix.
	MaxSuffixLength int

	// DefaultSuffixes are suffixes that should be added by default before the provided suffixes when Suffix is called.
	DefaultSuffixes []string
}

// WithDefaultSuffixes returns a new Namer with updated default suffixes.
func (n Namer) WithDefaultSuffixes(defaultSuffixes ...string) Namer {
	n.DefaultSuffixes = defaultSuffixes
	return n
}

// Suffix a resource name.
//
// Panics if the suffix exceeds the suffix limits.
// Trims the name if the concatenated result would exceed the limits.
func (n Namer) Suffix(ownerName string, suffixes ...string) string {
	var suffixBuilder strings.Builder
	for _, s := range n.DefaultSuffixes {
		suffixBuilder.WriteString("-") // #nosec G104
		suffixBuilder.WriteString(s)   // #nosec G104
	}
	for _, s := range suffixes {
		suffixBuilder.WriteString("-") // #nosec G104
		suffixBuilder.WriteString(s)   // #nosec G104
	}

	suffix := suffixBuilder.String()

	// This should never happen because we control all the suffixes!
	if len(suffix) > n.MaxSuffixLength {
		panic(fmt.Errorf("suffix should not exceed %d characters: got %s", n.MaxSuffixLength, suffix))
	}

	// This should never happen because the ownerName length should have been validated.
	// Trim the ownerName and log an error as fallback.
	maxPrefixLength := MaxNameLength - len(suffix)
	if len(ownerName) > maxPrefixLength {
		log.Error(fmt.Errorf("ownerName should not exceed %d characters: got %s", maxPrefixLength, ownerName),
			"Failed to suffix resource")
		ownerName = ownerName[:maxPrefixLength]
	}

	return stringsutil.Concat(ownerName, suffix)
}
