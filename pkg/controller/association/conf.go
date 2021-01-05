// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unsafe"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// FetchWithAssociations retrieves an object and extracts its association configurations.
func FetchWithAssociations(
	ctx context.Context,
	client k8s.Client,
	request reconcile.Request,
	associated commonv1.Associated,
) error {
	span, _ := apm.StartSpan(ctx, "fetch_associations", tracing.SpanTypeApp)
	defer span.End()

	if err := client.Get(context.Background(), request.NamespacedName, associated); err != nil {
		return err
	}

	for _, association := range associated.GetAssociations() {
		assocConf, err := GetAssociationConf(association)
		if err != nil {
			return err
		}
		association.SetAssociationConf(assocConf)
	}

	return nil
}

func AreConfiguredIfSet(associations []commonv1.Association, r record.EventRecorder) bool {
	allAssociationsConfigured := true
	for _, association := range associations {
		allAssociationsConfigured = allAssociationsConfigured && IsConfiguredIfSet(association, r)
	}
	return allAssociationsConfigured
}

// IsConfiguredIfSet checks if an association is set in the spec and if it has been configured by an association controller.
// This is used to prevent the deployment of an associated resource while the association is not yet fully configured.
func IsConfiguredIfSet(association commonv1.Association, r record.EventRecorder) bool {
	ref := association.AssociationRef()
	if (&ref).IsDefined() && !association.AssociationConf().IsConfigured() {
		r.Event(
			association,
			corev1.EventTypeWarning,
			events.EventAssociationError,
			fmt.Sprintf("Association backend for %s is not configured", association.AssociationType()),
		)
		log.Info("Association not established: skipping association resource reconciliation",
			"kind", association.GetObjectKind().GroupVersionKind().Kind,
			"namespace", association.GetNamespace(),
			"name", association.GetName(),
			"ref_namespace", ref.Namespace,
			"ref_name", ref.Name,
		)
		return false
	}
	return true
}

// ElasticsearchAuthSettings returns the user and the password to be used by an associated object to authenticate
// against an Elasticsearch cluster.
// This is also used for transitive authentication that relies on Elasticsearch native realm (eg. APMServer -> Kibana)
func ElasticsearchAuthSettings(c k8s.Client, association commonv1.Association) (username, password string, err error) {
	assocConf := association.AssociationConf()
	if !assocConf.AuthIsConfigured() {
		return "", "", nil
	}

	secretObjKey := types.NamespacedName{Namespace: association.GetNamespace(), Name: assocConf.AuthSecretName}
	var secret corev1.Secret
	if err := c.Get(context.Background(), secretObjKey, &secret); err != nil {
		return "", "", err
	}

	data, ok := secret.Data[assocConf.AuthSecretKey]
	if !ok {
		return "", "", errors.Errorf("auth secret key %s doesn't exist", assocConf.AuthSecretKey)
	}

	return assocConf.AuthSecretKey, string(data), nil
}

// AllowVersion returns true if the given resourceVersion is lower or equal to the associations' versions.
// For example: Kibana in version 7.8.0 cannot be deployed if its Elasticsearch association reports version 7.7.0.
// A difference in the patch version is ignored: Kibana 7.8.1+ can be deployed alongside Elasticsearch 7.8.0.
// Referenced resources version is parsed from the association conf annotation.
func AllowVersion(resourceVersion version.Version, associated commonv1.Associated, logger logr.Logger, recorder record.EventRecorder) bool {
	for _, assoc := range associated.GetAssociations() {
		assocRef := assoc.AssociationRef()
		if !assocRef.IsDefined() {
			// no association specified, move on
			continue
		}
		if assoc.AssociationConf() == nil || assoc.AssociationConf().Version == "" {
			// no conf reported yet, this may be the initial resource creation
			logger.Info("Delaying version deployment since the version of an associated resource is not reported yet",
				"version", resourceVersion, "ref_namespace", assocRef.Namespace, "ref_name", assocRef.Name)
			return false
		}
		refVer, err := version.Parse(assoc.AssociationConf().Version)
		if err != nil {
			logger.Error(err, "Invalid version found in association configuration", "association_version", assoc.AssociationConf().Version)
			return false
		}
		if !refVer.IsSameOrAfterIgnoringPatch(resourceVersion) {
			// the version of the referenced resource (example: Elasticsearch) is lower than
			// the desired version of the reconciled resource (example: Kibana)
			logger.Info("Delaying version deployment since a referenced resource is not upgraded yet",
				"version", resourceVersion, "ref_version", refVer,
				"ref_type", assoc.AssociationType(), "ref_namespace", assocRef.Namespace, "ref_name", assocRef.Name)
			recorder.Event(associated, corev1.EventTypeWarning, events.EventReasonDelayed,
				fmt.Sprintf("Delaying deployment of version %s since the referenced %s is not upgraded yet", resourceVersion, assoc.AssociationType()))
			return false
		}
	}
	return true
}

// GetAssociationConf extracts the association configuration from the given object by reading the annotations.
func GetAssociationConf(association commonv1.Association) (*commonv1.AssociationConf, error) {
	accessor := meta.NewAccessor()
	annotations, err := accessor.Annotations(association)
	if err != nil {
		return nil, err
	}

	return extractAssociationConf(annotations, association.AssociationConfAnnotationName())
}

func extractAssociationConf(annotations map[string]string, annotationName string) (*commonv1.AssociationConf, error) {
	if len(annotations) == 0 {
		return nil, nil
	}

	var assocConf commonv1.AssociationConf
	serializedConf, exists := annotations[annotationName]
	if !exists || serializedConf == "" {
		return nil, nil
	}

	if err := json.Unmarshal(unsafeStringToBytes(serializedConf), &assocConf); err != nil {
		return nil, errors.Wrapf(err, "failed to extract association configuration")
	}

	return &assocConf, nil
}

// RemoveObsoleteAssociationConfs removes all no longer needed annotations on `associated` matching
// `associationConfAnnotationNameBase` prefix.
func RemoveObsoleteAssociationConfs(
	client k8s.Client,
	associated commonv1.Associated,
	associationConfAnnotationNameBase string,
) error {
	accessor := meta.NewAccessor()
	annotations, err := accessor.Annotations(associated)
	if err != nil {
		return err
	}

	expected := make(map[string]bool)
	for _, association := range associated.GetAssociations() {
		expected[association.AssociationConfAnnotationName()] = true
	}

	modified := false
	for key := range annotations {
		if strings.HasPrefix(key, associationConfAnnotationNameBase) && !expected[key] {
			delete(annotations, key)
			modified = true
		}
	}

	if !modified {
		return nil
	}

	if err := accessor.SetAnnotations(associated, annotations); err != nil {
		return err
	}

	return client.Update(context.Background(), associated)
}

// RemoveAssociationConf removes the association configuration annotation.
func RemoveAssociationConf(client k8s.Client, association commonv1.Association) error {
	associated := association.Associated()
	accessor := meta.NewAccessor()
	annotations, err := accessor.Annotations(associated)
	if err != nil {
		return err
	}

	if len(annotations) == 0 {
		return nil
	}

	annotationName := association.AssociationConfAnnotationName()
	if _, exists := annotations[annotationName]; !exists {
		return nil
	}

	delete(annotations, annotationName)
	if err := accessor.SetAnnotations(associated, annotations); err != nil {
		return err
	}

	return client.Update(context.Background(), associated)
}

// UpdateAssociationConf updates the association configuration annotation.
func UpdateAssociationConf(
	client k8s.Client,
	association commonv1.Association,
	wantConf *commonv1.AssociationConf,
) error {
	accessor := meta.NewAccessor()

	obj := association.Associated()
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

	annotationName := association.AssociationConfAnnotationName()
	annotations[annotationName] = unsafeBytesToString(serializedConf)
	if err := accessor.SetAnnotations(obj, annotations); err != nil {
		return err
	}

	// persist the changes
	return client.Update(context.Background(), obj)
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
