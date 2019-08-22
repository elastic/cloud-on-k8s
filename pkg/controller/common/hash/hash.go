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
	// TemplateHashLabelName is a label to annotate a Kubernetes resource
	// with the hash of its initial template before creation.
	TemplateHashLabelName = "common.k8s.elastic.co/template-hash"
)

// SetTemplateHashLabel adds a label containing the hash of the given template into the
// given labels. This label can then be used for template comparisons.
func SetTemplateHashLabel(labels map[string]string, template interface{}) map[string]string {
	return SetHashLabel(TemplateHashLabelName, labels, template)
}

func SetHashLabel(labelName string, labels map[string]string, template interface{}) map[string]string {
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

// HashObject writes the specified object to a hash using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
// The returned hash can be used for object comparisons.
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
