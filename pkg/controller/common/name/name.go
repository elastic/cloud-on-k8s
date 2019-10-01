// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package name

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
	"github.com/hashicorp/go-multierror"
	"k8s.io/apimachinery/pkg/util/validation"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// MaxResourceNameLength is the max length of a name that will keep all its child resources under the K8S max label
	// length after accounting for the longest suffix that could be added by the operator. See https://git.io/fjpFl.
	MaxResourceNameLength = 36
	// MaxSuffixLength is the max allowed suffix length that will keep a name within K8S label length restrictions.
	MaxSuffixLength = validation.LabelValueMaxLength - MaxResourceNameLength
)

var log = logf.Log.WithName("name")

// nameLengthError is an error type for names exceeding the allowed length.
type nameLengthError struct {
	reason    string
	maxLen    int
	actualLen int
	value     string
}

func newNameLengthError(reason string, maxLen int, value string) nameLengthError {
	return nameLengthError{
		reason:    reason,
		maxLen:    maxLen,
		actualLen: len(value),
		value:     value,
	}
}

func (nle nameLengthError) Error() string {
	return fmt.Sprintf("%s: '%s' has length %d which is more than %d", nle.reason, nle.value, nle.actualLen, nle.maxLen)
}

// Namer assists with constructing names for K8s resources and avoiding collisions by ensuring that certain suffixes
// are always used, and prevents the use of too long suffixes.
type Namer struct {
	MaxSuffixLength int
	MaxNameLength   int
	DefaultSuffixes []string
}

// NewNamer creates a new Namer object with the default suffix length restriction.
func NewNamer(defaultSuffixes ...string) Namer {
	return Namer{
		MaxSuffixLength: MaxSuffixLength,
		MaxNameLength:   validation.DNS1123SubdomainMaxLength,
		DefaultSuffixes: defaultSuffixes,
	}
}

// WithDefaultSuffixes returns a new Namer with updated default suffixes.
func (n Namer) WithDefaultSuffixes(defaultSuffixes ...string) Namer {
	n.DefaultSuffixes = defaultSuffixes
	return n
}

// Suffix generates a resource name by appending the specified suffixes.
func (n Namer) Suffix(ownerName string, suffixes ...string) string {
	suffixedName, err := n.SafeSuffix(ownerName, suffixes...)
	if err != nil {
		// we should never encounter an error at this point because the names should have been validated
		log.Error(err, "Invalid name. This could prevent the operator from functioning correctly", "name", suffixedName)
	}

	return suffixedName
}

// SafeSuffix attempts to generate a suffixed name, returning an error if the generated name is unacceptable.
func (n Namer) SafeSuffix(ownerName string, suffixes ...string) (string, error) {
	var suffixBuilder strings.Builder
	var err error

	for _, s := range n.DefaultSuffixes {
		suffixBuilder.WriteString("-")
		suffixBuilder.WriteString(s)
	}
	for _, s := range suffixes {
		suffixBuilder.WriteString("-")
		suffixBuilder.WriteString(s)
	}

	suffix := suffixBuilder.String()

	// This should never happen because we control all the suffixes!
	if len(suffix) > n.MaxSuffixLength {
		err = multierror.Append(err, newNameLengthError("suffix exceeds max length", n.MaxSuffixLength, suffix))
		suffix = truncate(suffix, n.MaxSuffixLength)
	}

	maxPrefixLength := n.MaxNameLength - len(suffix)
	if len(ownerName) > maxPrefixLength {
		err = multierror.Append(err, newNameLengthError("owner name exceeds max length", maxPrefixLength, ownerName))
		ownerName = truncate(ownerName, maxPrefixLength)
	}

	return stringsutil.Concat(ownerName, suffix), err
}

func truncate(s string, length int) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len()+utf8.RuneLen(r) > length {
			return b.String()
		}
		b.WriteRune(r)
	}

	return b.String()
}
