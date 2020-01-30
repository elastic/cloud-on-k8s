// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"context"
	"encoding/json"
	"reflect"
	"unsafe"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/pkg/errors"
	"go.elastic.co/apm"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// FetchWithAssociation retrieves an object and extracts its association configuration.
func FetchWithAssociation(ctx context.Context, client k8s.Client, request reconcile.Request, obj commonv1.Associator) error {
	span, _ := apm.StartSpan(ctx, "fetch_association", tracing.SpanTypeApp)
	defer span.End()
	if err := client.Get(request.NamespacedName, obj); err != nil {
		return err
	}

	assocConf, err := GetAssociationConf(obj)
	if err != nil {
		return err
	}

	obj.SetAssociationConf(assocConf)
	return nil
}

// GetAssociationConf extracts the association configuration from the given object by reading the annotations.
func GetAssociationConf(obj runtime.Object) (*commonv1.AssociationConf, error) {
	accessor := meta.NewAccessor()
	annotations, err := accessor.Annotations(obj)
	if err != nil {
		return nil, err
	}

	return extractAssociationConf(annotations)
}

func extractAssociationConf(annotations map[string]string) (*commonv1.AssociationConf, error) {
	if len(annotations) == 0 {
		return nil, nil
	}

	var assocConf commonv1.AssociationConf
	serializedConf, exists := annotations[annotation.AssociationConfAnnotation]
	if !exists || serializedConf == "" {
		return nil, nil
	}

	if err := json.Unmarshal(unsafeStringToBytes(serializedConf), &assocConf); err != nil {
		return nil, errors.Wrapf(err, "failed to extract association configuration")
	}

	return &assocConf, nil
}

// RemoveAssociationConf removes the association configuration annotation.
func RemoveAssociationConf(client k8s.Client, obj runtime.Object) error {
	accessor := meta.NewAccessor()
	annotations, err := accessor.Annotations(obj)
	if err != nil {
		return err
	}

	if len(annotations) == 0 {
		return nil
	}

	if _, exists := annotations[annotation.AssociationConfAnnotation]; !exists {
		return nil
	}

	delete(annotations, annotation.AssociationConfAnnotation)
	if err := accessor.SetAnnotations(obj, annotations); err != nil {
		return err
	}

	return client.Update(obj)
}

// UpdateAssociationConf updates the association configuration annotation.
func UpdateAssociationConf(client k8s.Client, obj runtime.Object, wantConf *commonv1.AssociationConf) error {
	accessor := meta.NewAccessor()
	annotations, err := accessor.Annotations(obj)
	if err != nil {
		return err
	}

	// serialize the config and update the object
	serializedConf, err := json.Marshal(wantConf)
	if err != nil {
		return errors.Wrapf(err, "failed to serialize configuration")
	}

	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[annotation.AssociationConfAnnotation] = unsafeBytesToString(serializedConf)
	if err := accessor.SetAnnotations(obj, annotations); err != nil {
		return err
	}

	// persist the changes
	return client.Update(obj)
}

// unsafeStringToBytes converts a string to a byte array without making extra allocations.
// since we read potentially large strings from annotations on every reconcile loop, this should help
// reduce the amount of garbage created.
func unsafeStringToBytes(s string) []byte {
	hdr := *(*reflect.StringHeader)(unsafe.Pointer(&s))
	return *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Data: hdr.Data,
		Len:  hdr.Len,
		Cap:  hdr.Len,
	}))
}

// unsafeBytesToString converts a byte array to string without making extra allocations.
func unsafeBytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
