// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package rolemapping

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const (
	// PKIRoleMappingRequestType identifies a Secret containing an Elasticsearch PKI role mapping request.
	PKIRoleMappingRequestType = "pki-role-mapping-request"

	// PublicKeyFingerprintField binds the request to the SHA-256 digest of the client certificate SPKI.
	PublicKeyFingerprintField = "publicKeyFingerprint"
	// RolesField contains the comma-separated Elasticsearch roles granted to the certificate public key.
	RolesField = "roles"
	// ResourceUIDField contains the Kubernetes UID of the associated resource that owns this request.
	ResourceUIDField = "resourceUID"
	// AssociationNameField contains the ECK association name (e.g. kb-es) identifying the association type.
	AssociationNameField = "associationName"
	// ResourceNamespaceField and ResourceNameField identify the associated resource that owns this request.
	// They are only used to make conflict messages actionable, so they are not required.
	ResourceNamespaceField = "resourceNamespace"
	ResourceNameField      = "resourceName"

	// PKIConflictAnnotation is set by ReconcilePKIRoleMappings on a request Secret whose role mapping is withheld:
	// its fingerprint is shared by requests asking for different roles, or its name is used by a user-defined role
	// mapping. The value is the conflict description; it is removed once the conflict is resolved.
	PKIConflictAnnotation = "association.k8s.elastic.co/pki-conflict"

	fieldNotFound = "field %s not found in PKI role mapping request Secret %s/%s"
)

// Request contains the data needed to build one Elasticsearch PKI role mapping.
type Request struct {
	SecretNamespace      string
	SecretName           string
	PublicKeyFingerprint string
	Roles                string
	ResourceUID          string
	AssociationName      string
	// ResourceNamespace and ResourceName identify the associated resource that owns this request, if set.
	ResourceNamespace string
	ResourceName      string
	// ConflictAnnotation is the current value of PKIConflictAnnotation on the request Secret,
	// carried here so patchConflictAnnotations can skip writes when the annotation is already correct.
	ConflictAnnotation string
}

// RequestLabels returns labels matching PKI role mapping request Secrets for es.
func RequestLabels(es esv1.Elasticsearch) map[string]string {
	return map[string]string{
		label.ClusterNameLabelName: es.Name,
		commonv1.TypeLabelName:     PKIRoleMappingRequestType,
	}
}

// RetrieveRequests returns all PKI role mapping requests for es.
func RetrieveRequests(ctx context.Context, c k8s.Client, es esv1.Elasticsearch) ([]Request, error) {
	var secrets corev1.SecretList
	if err := c.List(ctx, &secrets,
		client.InNamespace(es.Namespace),
		client.MatchingLabels(RequestLabels(es)),
	); err != nil {
		return nil, err
	}

	requests := make([]Request, 0, len(secrets.Items))
	for i := range secrets.Items {
		request, err := parseRequestSecret(secrets.Items[i])
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, nil
}

func parseRequestSecret(secret corev1.Secret) (Request, error) {
	request := Request{
		SecretNamespace: secret.Namespace,
		SecretName:      secret.Name,
	}
	var err error
	request.Roles = strings.TrimSpace(string(secret.Data[RolesField]))
	if request.Roles == "" {
		err = errors.Join(err, fmt.Errorf(fieldNotFound, RolesField, secret.Namespace, secret.Name))
	}
	request.PublicKeyFingerprint = string(secret.Data[PublicKeyFingerprintField])
	if request.PublicKeyFingerprint == "" {
		err = errors.Join(err, fmt.Errorf(fieldNotFound, PublicKeyFingerprintField, secret.Namespace, secret.Name))
	}
	request.ResourceUID = string(secret.Data[ResourceUIDField])
	if request.ResourceUID == "" {
		err = errors.Join(err, fmt.Errorf(fieldNotFound, ResourceUIDField, secret.Namespace, secret.Name))
	}
	request.AssociationName = string(secret.Data[AssociationNameField])
	if request.AssociationName == "" {
		err = errors.Join(err, fmt.Errorf(fieldNotFound, AssociationNameField, secret.Namespace, secret.Name))
	}
	request.ResourceNamespace = string(secret.Data[ResourceNamespaceField])
	request.ResourceName = string(secret.Data[ResourceNameField])
	request.ConflictAnnotation = secret.Annotations[PKIConflictAnnotation]
	return request, err
}
