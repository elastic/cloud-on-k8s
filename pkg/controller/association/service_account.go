// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"go.elastic.co/apm"
	"golang.org/x/crypto/pbkdf2"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	Namespace string = "elastic"

	ServiceAccountNameField       = "serviceAccount"
	ServiceAccountTokenValueField = "token"
)

func applicationSecretLabels(es esv1.Elasticsearch) map[string]string {
	return common.AddCredentialsLabel(map[string]string{
		label.ClusterNamespaceLabelName: es.Namespace,
		label.ClusterNameLabelName:      es.Name,
	})
}

func esSecretsLabels(es esv1.Elasticsearch) map[string]string {
	return map[string]string{
		label.ClusterNamespaceLabelName: es.Namespace,
		label.ClusterNameLabelName:      es.Name,
		common.TypeLabelName:            esuser.ServiceAccountTokenType,
	}
}

// reconcileApplicationSecret reconciles the Secret which contains the application token.
func reconcileApplicationSecret(
	ctx context.Context,
	client k8s.Client,
	es esv1.Elasticsearch,
	applicationSecretName types.NamespacedName,
	commonLabels map[string]string,
	tokenName string,
	serviceAccount commonv1.ServiceAccountName,
) (*Token, error) {
	span, _ := apm.StartSpan(ctx, "reconcile_sa_token_application", tracing.SpanTypeApp)
	defer span.End()

	applicationStore := corev1.Secret{}
	err := client.Get(context.Background(), applicationSecretName, &applicationStore)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, err
	}

	var token *Token
	if k8serrors.IsNotFound(err) || len(applicationStore.Data) == 0 {
		// Secret does not exist or is empty, create a new token
		token, err = newApplicationToken(serviceAccount, tokenName)
		if err != nil {
			return nil, err
		}
	} else {
		// Attempt to read current token, create a new one in case of an error.
		token, err = getOrCreateToken(&es, applicationSecretName.Name, applicationStore.Data, serviceAccount, tokenName)
		if err != nil {
			return nil, err
		}
	}

	labels := applicationSecretLabels(es)
	for labelName, labelValue := range commonLabels {
		labels[labelName] = labelValue
	}
	applicationStore = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      applicationSecretName.Name,
			Namespace: applicationSecretName.Namespace,
			Labels:    labels,
		},
		Data: map[string][]byte{
			esuser.ServiceAccountTokenNameField: []byte(token.TokenName),
			ServiceAccountTokenValueField:       []byte(token.Token),
			esuser.ServiceAccountHashField:      []byte(token.Hash),
			ServiceAccountNameField:             []byte(token.ServiceAccountName),
		},
	}

	if _, err := reconciler.ReconcileSecret(client, applicationStore, nil); err != nil {
		return nil, err
	}

	return token, err
}

func getOrCreateToken(
	es *esv1.Elasticsearch,
	secretName string,
	secretData map[string][]byte,
	serviceAccountName commonv1.ServiceAccountName,
	tokenName string,
) (*Token, error) {
	token := getCurrentApplicationToken(es, secretName, secretData)
	if token == nil {
		// We need to create a new token
		return newApplicationToken(serviceAccountName, tokenName)
	}
	return token, nil
}

// reconcileElasticsearchSecret ensures the Secret for Elasticsearch exists and hold the expected token.
func reconcileElasticsearchSecret(
	ctx context.Context,
	client k8s.Client,
	es esv1.Elasticsearch,
	elasticsearchSecretName types.NamespacedName,
	commonLabels map[string]string,
	token Token,
) error {
	span, _ := apm.StartSpan(ctx, "reconcile_sa_token_elasticsearch", tracing.SpanTypeApp)
	defer span.End()
	fullyQualifiedName := token.ServiceAccountName + "/" + token.TokenName
	labels := esSecretsLabels(es)
	for labelName, labelValue := range commonLabels {
		labels[labelName] = labelValue
	}
	esSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      elasticsearchSecretName.Name,
			Namespace: elasticsearchSecretName.Namespace,
			Labels:    labels,
		},
		Data: map[string][]byte{
			esuser.ServiceAccountTokenNameField: []byte(fullyQualifiedName),
			esuser.ServiceAccountHashField:      []byte(token.Hash),
		},
	}
	_, err := reconciler.ReconcileSecret(client, esSecret, &es)
	return err
}

func ReconcileServiceAccounts(
	ctx context.Context,
	client k8s.Client,
	es esv1.Elasticsearch,
	commonLabels map[string]string,
	applicationSecretName types.NamespacedName,
	elasticsearchSecretName types.NamespacedName,
	serviceAccount commonv1.ServiceAccountName,
	applicationName string,
	applicationUID types.UID,
) error {
	tokenName := tokenName(applicationSecretName.Namespace, applicationName, applicationUID)
	token, err := reconcileApplicationSecret(ctx, client, es, applicationSecretName, commonLabels, tokenName, serviceAccount)
	if err != nil {
		return err
	}
	return reconcileElasticsearchSecret(ctx, client, es, elasticsearchSecretName, commonLabels, *token)
}

// getCurrentApplicationToken returns the current token from the application Secret, or nil if the content of the Secret is not valid.
func getCurrentApplicationToken(es *esv1.Elasticsearch, secretName string, secretData map[string][]byte) *Token {
	if len(secretData) == 0 {
		log.V(1).Info("secret is empty", "es_name", es.Name, "namespace", es.Namespace, "secret", secretName)
		return nil
	}
	result := &Token{}
	if value := getFieldOrNil(es, secretName, secretData, esuser.ServiceAccountTokenNameField); value != nil && len(*value) > 0 {
		result.TokenName = *value
	} else {
		return nil
	}

	if value := getFieldOrNil(es, secretName, secretData, ServiceAccountTokenValueField); value != nil && len(*value) > 0 {
		result.Token = *value
	} else {
		return nil
	}

	if value := getFieldOrNil(es, secretName, secretData, esuser.ServiceAccountHashField); value != nil && len(*value) > 0 {
		result.Hash = *value
	} else {
		return nil
	}

	if value := getFieldOrNil(es, secretName, secretData, ServiceAccountNameField); value != nil && len(*value) > 0 {
		result.ServiceAccountName = *value
	} else {
		return nil
	}

	return result
}

func getFieldOrNil(es *esv1.Elasticsearch, secretName string, secretData map[string][]byte, fieldName string) *string {
	data, exists := secretData[fieldName]
	if !exists {
		log.V(1).Info(fmt.Sprintf("%s field is missing in service account token Secret", fieldName), "es_name", es.Name, "namespace", es.Namespace, "secret", secretName)
		return nil
	}
	fieldValue := string(data)
	return &fieldValue
}

var prefix = [...]byte{0x0, 0x1, 0x0, 0x1}

// newApplicationToken generates a new token for a given service account.
func newApplicationToken(serviceAccountName commonv1.ServiceAccountName, tokenName string) (*Token, error) {
	secret := common.RandomBytes(64)
	hash, err := pbkdf2Key(secret)
	if err != nil {
		return nil, err
	}

	fullyQualifiedName := fmt.Sprintf("%s/%s", Namespace, serviceAccountName)
	suffix := []byte(fmt.Sprintf("%s/%s:%s", fullyQualifiedName, tokenName, secret))
	token := base64.StdEncoding.EncodeToString(append(prefix[:], suffix...))

	return &Token{
		ServiceAccountName: fullyQualifiedName,
		TokenName:          tokenName,
		Token:              token,
		Hash:               hash,
	}, nil
}

func tokenName(
	applicationNamespace, applicationName string,
	applicationUID types.UID,
) string {
	return fmt.Sprintf("%s_%s_%s", applicationNamespace, applicationName, applicationUID)
}

// Token stores all the required data for a given service account token.
type Token struct {
	ServiceAccountName string
	TokenName          string
	Token              string
	Hash               string
}

func (u Token) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		ServiceAccountName string `json:"serviceAccountName"`
		TokenName          string `json:"tokenName"`
		Token              string `json:"token"`
		Hash               string `json:"hash"`
	}{
		ServiceAccountName: u.ServiceAccountName,
		TokenName:          u.TokenName,
		Token:              "REDACTED",
		Hash:               "REDACTED",
	})
}

// -- crypto

const (
	pbkdf2StretchPrefix     = "{PBKDF2_STRETCH}"
	pbkdf2DefaultCost       = 10000
	pbkdf2KeyLength         = 32
	pbkdf2DefaultSaltLength = 32
)

// pbkdf2Key derives a key from the provided secret, as expected by the service tokens file store in Elasticsearch.
func pbkdf2Key(secret []byte) (string, error) {
	var result strings.Builder
	result.WriteString(pbkdf2StretchPrefix)
	result.WriteString(strconv.Itoa(pbkdf2DefaultCost))
	result.WriteString("$")

	salt := make([]byte, pbkdf2DefaultSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	result.WriteString(base64.StdEncoding.EncodeToString(salt))
	result.WriteString("$")

	hashedSecret := sha512.Sum512(secret)
	hashedSecretAsString := hex.EncodeToString(hashedSecret[:])

	dk := pbkdf2.Key([]byte(hashedSecretAsString), salt, pbkdf2DefaultCost, pbkdf2KeyLength, sha512.New)
	result.WriteString(base64.StdEncoding.EncodeToString(dk))
	return result.String(), nil
}
