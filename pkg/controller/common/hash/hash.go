// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package hash

import (
	"fmt"
	"hash"
	"hash/fnv"

	"github.com/davecgh/go-spew/spew"
)

const (
	// TemplateHashLabelName is a label to annotate a Kubernetes resource
	// with the hash of its initial template before creation.
	TemplateHashLabelName = "common.k8s.elastic.co/template-hash"
)

// SetTemplateHashLabel adds a label containing the hash of the given template into the
// given labels. This label can then be used for template comparisons.
func SetTemplateHashLabel(labels map[string]string, template interface{}) map[string]string {
	return setHashLabel(TemplateHashLabelName, labels, template)
}

func setHashLabel(labelName string, labels map[string]string, template interface{}) map[string]string {
	if labels == nil {
		labels = map[string]string{}
	}
	labels[labelName] = HashObject(template)
	return labels
}

// GetTemplateHashLabel returns the template hash label value if set, or an empty string.
func GetTemplateHashLabel(labels map[string]string) string {
	return labels[TemplateHashLabelName]
}

// HashObject returns a hash of a given object using the 32-bit FNV-1 hash function
// and the spew library to print the object (see WriteHashObject).
// This is inspired by controller revisions in StatefulSets:
// https://github.com/kubernetes/kubernetes/blob/8de1569ddae62e8fab559fe6bd210a5d6100a277/pkg/controller/history/controller_history.go#L89-L101
func HashObject(object interface{}) string {
	objHash := fnv.New32a()
	WriteHashObject(objHash, object)
	return fmt.Sprint(objHash.Sum32())
}

// WriteHashObject writes specified object to hash using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
// The hash can be used for object comparisons.
// Copy of https://github.com/kubernetes/kubernetes/blob/ea0764452222146c47ec826977f49d7001b0ea8c/pkg/util/hash/hash.go#L28
func WriteHashObject(hasher hash.Hash, objectToWrite interface{}) {
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}
	printer.Fprintf(hasher, "%#v", objectToWrite)
}
