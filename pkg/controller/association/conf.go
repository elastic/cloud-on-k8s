// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unsafe"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func AreConfiguredIfSet(associations []commonv1.Association, r record.EventRecorder) (bool, error) {
	allAssociationsConfigured := true
	for _, association := range associations {
		isAssocConfigured, err := IsConfiguredIfSet(association, r)
		if err != nil {
			return false, err
		}
		allAssociationsConfigured = allAssociationsConfigured && isAssocConfigured
	}
	return allAssociationsConfigured, nil
}

// IsConfiguredIfSet checks if an association is set in the spec and if it has been configured by an association controller.
// This is used to prevent the deployment of an associated resource while the association is not yet fully configured.
func IsConfiguredIfSet(association commonv1.Association, r record.EventRecorder) (bool, error) {
	ref := association.AssociationRef()
	assocConf, err := association.AssociationConf()
	if err != nil {
		return false, err
	}
	if (&ref).IsDefined() && !assocConf.IsConfigured() {
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
		return false, nil
	}
	return true, nil
}

type Credentials struct {
	Username, Password, ServiceAccountToken string
}

func (c Credentials) HasServiceAccountToken() bool {
	return len(c.ServiceAccountToken) > 0
}

// ElasticsearchAuthSettings returns the credentials to be used by an associated object to authenticate
// against an Elasticsearch cluster.
// This is also used for transitive authentication that relies on Elasticsearch native realm (eg. APMServer -> Kibana)
func ElasticsearchAuthSettings(c k8s.Client, association commonv1.Association) (Credentials, error) {
	assocConf, err := association.AssociationConf()
	if err != nil {
		return Credentials{}, err
	}
	if !assocConf.AuthIsConfigured() {
		return Credentials{}, nil
	}

	secretObjKey := types.NamespacedName{Namespace: association.GetNamespace(), Name: assocConf.AuthSecretName}
	var secret corev1.Secret
	if err := c.Get(context.Background(), secretObjKey, &secret); err != nil {
		return Credentials{}, err
	}

	data, ok := secret.Data[assocConf.AuthSecretKey]
	if !ok {
		return Credentials{}, errors.Errorf("auth secret key %s doesn't exist", assocConf.AuthSecretKey)
	}

	if assocConf.IsServiceAccount {
		return Credentials{ServiceAccountToken: string(data)}, nil
	}

	return Credentials{Username: assocConf.AuthSecretKey, Password: string(data)}, nil
}

// AllowVersion returns true if the given resourceVersion is lower or equal to the associations' versions.
// For example: Kibana in version 7.8.0 cannot be deployed if its Elasticsearch association reports version 7.7.0.
// A difference in the patch version is ignored: Kibana 7.8.1+ can be deployed alongside Elasticsearch 7.8.0.
// Referenced resources version is parsed from the association conf annotation.
func AllowVersion(resourceVersion version.Version, associated commonv1.Associated, logger logr.Logger, recorder record.EventRecorder) (bool, error) {
	for _, assoc := range associated.GetAssociations() {
		assocRef := assoc.AssociationRef()
		if !assocRef.IsDefined() {
			// no association specified, move on
			continue
		}
		assocConf, err := assoc.AssociationConf()
		if err != nil {
			return false, err
		}
		if assocConf == nil || assocConf.Version == "" {
			// no conf reported yet, this may be the initial resource creation
			logger.Info("Delaying version deployment since the version of an associated resource is not reported yet",
				"version", resourceVersion, "ref_namespace", assocRef.Namespace, "ref_name", assocRef.Name)
			return false, nil
		}
		refVer, err := version.Parse(assocConf.Version)
		if err != nil {
			logger.Error(err, "Invalid version found in association configuration", "association_version", assocConf.Version)
			return false, nil
		}

		compatibleVersions := refVer.GTE(resourceVersion) || ((refVer.Major == resourceVersion.Major) && (refVer.Minor == resourceVersion.Minor))
		if !compatibleVersions {
			// the version of the referenced resource (example: Elasticsearch) is lower than
			// the desired version of the reconciled resource (example: Kibana)
			logger.Info("Delaying version deployment since a referenced resource is not upgraded yet",
				"version", resourceVersion, "ref_version", refVer,
				"ref_type", assoc.AssociationType(), "ref_namespace", assocRef.Namespace, "ref_name", assocRef.Name)
			recorder.Event(associated, corev1.EventTypeWarning, events.EventReasonDelayed,
				fmt.Sprintf("Delaying deployment of version %s since the referenced %s is not upgraded yet", resourceVersion, assoc.AssociationType()))
			return false, nil
		}
	}
	return true, nil
}

// SingleAssociationOfType returns single association from the provided slice that matches provided type. Returns
// nil if such association can't be found. Returns an error if more than one association matches the type.
func SingleAssociationOfType(
	associations []commonv1.Association,
	associationType commonv1.AssociationType,
) (commonv1.Association, error) {
	var result commonv1.Association
	for _, assoc := range associations {
		if assoc.AssociationType() == associationType {
			if result != nil {
				return nil, fmt.Errorf("more than one association of type %s among %d associations", associationType, len(associations))
			}

			result = assoc
		}
	}

	return result, nil
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

// unsafeBytesToString converts a byte array to string without making extra allocations.
func unsafeBytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
