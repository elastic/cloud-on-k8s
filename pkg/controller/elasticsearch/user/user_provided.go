// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package user

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user/filerealm"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// UserProvidedFileRealmWatchName returns the watch registered for user-provided file realm secrets.
func UserProvidedFileRealmWatchName(es types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-user-file-realm", es.Namespace, es.Name)
}

// UserProvidedRolesWatchName returns the watch registered for user-provided roles secrets.
func UserProvidedRolesWatchName(es types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-user-roles", es.Namespace, es.Name)
}

// reconcileUserProvidedFileRealm returns the aggregate file realm from the referenced sources in the es spec.
// It also ensures referenced secrets are watched for future reconciliations to be triggered on any change.
func reconcileUserProvidedFileRealm(
	c k8s.Client,
	es esv1.Elasticsearch,
	watched watches.DynamicWatches,
	recorder record.EventRecorder,
) (filerealm.Realm, error) {
	esKey := k8s.ExtractNamespacedName(&es)
	secretNames := make([]string, 0, len(es.Spec.Auth.FileRealm))
	for _, secretRef := range es.Spec.Auth.FileRealm {
		if secretRef.SecretName == "" {
			continue
		}
		secretNames = append(secretNames, secretRef.SecretName)
	}
	if err := watches.WatchUserProvidedSecrets(
		esKey,
		watched,
		UserProvidedFileRealmWatchName(esKey),
		secretNames,
	); err != nil {
		return filerealm.Realm{}, err
	}
	return retrieveUserProvidedFileRealm(c, es, recorder)
}

// reconcileUserProvidedRoles returns aggregate roles from the referenced sources in the es spec.
// It also ensures referenced secrets are watched for future reconciliations to be triggered on any change.
func reconcileUserProvidedRoles(
	c k8s.Client,
	es esv1.Elasticsearch,
	watched watches.DynamicWatches,
	recorder record.EventRecorder,
) (RolesFileContent, error) {
	esKey := k8s.ExtractNamespacedName(&es)
	secretNames := make([]string, 0, len(es.Spec.Auth.Roles))
	for _, secretRef := range es.Spec.Auth.Roles {
		if secretRef.SecretName == "" {
			continue
		}
		secretNames = append(secretNames, secretRef.SecretName)
	}
	if err := watches.WatchUserProvidedSecrets(
		esKey,
		watched,
		UserProvidedRolesWatchName(esKey),
		secretNames,
	); err != nil {
		return RolesFileContent{}, err
	}
	return retrieveUserProvidedRoles(c, es, recorder)
}

// retrieveUserProvidedRoles returns roles parsed from user-provided secrets specified in the es spec.
func retrieveUserProvidedRoles(
	c k8s.Client,
	es esv1.Elasticsearch,
	recorder record.EventRecorder,
) (RolesFileContent, error) {
	roles := make(RolesFileContent)
	for _, roleSource := range es.Spec.Auth.Roles {
		if roleSource.SecretName == "" {
			continue
		}
		var secret corev1.Secret
		secretRef := types.NamespacedName{Namespace: es.Namespace, Name: roleSource.SecretName}
		err := c.Get(context.Background(), secretRef, &secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				handleSecretNotFound(recorder, es, roleSource.SecretName)
				continue
			}
			return RolesFileContent{}, err
		}

		parsed, err := parseRolesFileContent(k8s.GetSecretEntry(secret, RolesFile))
		if err != nil {
			handleInvalidSecretData(recorder, es, roleSource.SecretName, err)
			continue
		}
		roles = roles.MergeWith(parsed)
	}
	return roles, nil
}

// retrieveUserProvidedFileRealm builds a Realm from aggregated user-provided secrets specified in the es spec.
func retrieveUserProvidedFileRealm(c k8s.Client, es esv1.Elasticsearch, recorder record.EventRecorder) (filerealm.Realm, error) {
	aggregated := filerealm.New()
	for _, fileRealmSource := range es.Spec.Auth.FileRealm {
		if fileRealmSource.SecretName == "" {
			continue
		}
		var secret corev1.Secret
		if err := c.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: fileRealmSource.SecretName}, &secret); err != nil {
			if apierrors.IsNotFound(err) {
				handleSecretNotFound(recorder, es, fileRealmSource.SecretName)
				continue
			}
			return filerealm.Realm{}, err
		}
		realm, err := filerealm.FromSecret(secret)
		if err != nil {
			handleInvalidSecretData(recorder, es, fileRealmSource.SecretName, err)
			continue
		}
		aggregated = aggregated.MergeWith(realm)
	}
	return aggregated, nil
}

func handleSecretNotFound(recorder record.EventRecorder, es esv1.Elasticsearch, secretName string) {
	msg := "referenced secret not found"
	// logging with info level since this may be expected if the secret is not in the cache yet
	log.Info(msg, "namespace", es.Namespace, "es_name", es.Name, "secret_name", secretName)
	recorder.Event(&es, corev1.EventTypeWarning, events.EventReasonUnexpected, msg+": "+secretName)
}

func handleInvalidSecretData(recorder record.EventRecorder, es esv1.Elasticsearch, secretName string, err error) {
	msg := "invalid data in secret"
	log.Error(err, msg, "namespace", es.Namespace, "es_name", es.Name, "secret_name", secretName)
	recorder.Event(&es, corev1.EventTypeWarning, events.EventReasonUnexpected, msg+": "+secretName)
}
