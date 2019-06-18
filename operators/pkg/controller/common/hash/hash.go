// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package hash

import (
	"fmt"
	"hash/fnv"

	"github.com/davecgh/go-spew/spew"
)

const (
	// SpecHashLabelName is a label to annotate a Kubernetes resource
	// with the hash of its initial spec before creation.
	SpecHashLabelName = "common.k8s.elastic.co/spec-hash"
)

// SetSpecHashLabel adds a label containing the hash of the given spec into the
// given labels. This label can then be used for spec comparison purpose.
func SetSpecHashLabel(labels map[string]string, spec interface{}) map[string]string {
	if labels == nil {
		labels = map[string]string{}
	}
	labels[SpecHashLabelName] = HashObject(spec)
	return labels
}

// GetSpecHashLabel returns the spec hash label value if set, or an empty string.
func GetSpecHashLabel(labels map[string]string) string {
	return labels[SpecHashLabelName]
}

// HashObject writes the specified object to a hash using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
// The returned hash can be used for object comparison purpose.
//
// This is inspired by controller revisions in StatefulSets:
// https://github.com/kubernetes/kubernetes/blob/8de1569ddae62e8fab559fe6bd210a5d6100a277/pkg/controller/history/controller_history.go#L89-L101
func HashObject(object interface{}) string {
	hf := fnv.New32()
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}
	_, _ = printer.Fprintf(hf, "%#v", object)
	return fmt.Sprint(hf.Sum32())
}
