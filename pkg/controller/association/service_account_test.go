// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/pbkdf2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var (
	existingElasticsearch = esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "elasticsearch-sample",
			Namespace:       "e2e-mercury",
			UID:             types.UID("eda4b94f-687d-4797-af3b-e46b248b82af"),
			ResourceVersion: "4242",
			Generation:      3,
		},
	}

	existingKibana = kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "kibana-sample",
			Namespace:       "e2e-venus",
			UID:             types.UID("892ff7d8-9cf2-48f0-89bc-5a530e77a930"),
			ResourceVersion: "8819",
			Generation:      2,
		},
		Spec: kbv1.KibanaSpec{
			Count: 1,
			ElasticsearchRef: commonv1.ObjectSelector{
				Name:      "elasticsearch-sample",
				Namespace: "e2e-mercury",
			},
		},
	}

	expectedKibanaUserSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "e2e-venus",
			Name:      "kibana-sample-kibana-user",
			UID:       types.UID("6f5cb31d-69c4-409d-8b8d-8eafafc6bbd7"),
			Labels: map[string]string{
				"eck.k8s.elastic.co/credentials":                 "true",
				"elasticsearch.k8s.elastic.co/cluster-name":      "elasticsearch-sample",
				"elasticsearch.k8s.elastic.co/cluster-namespace": "e2e-mercury",
				"kibanaassociation.k8s.elastic.co/name":          "kibana-sample",
				"kibanaassociation.k8s.elastic.co/namespace":     "e2e-venus",
				"kibanaassociation.k8s.elastic.co/type":          "elasticsearch",
			},
			ResourceVersion: "3442951",
		},
		Data: map[string][]byte{
			"serviceAccount": []byte("elastic/kibana"),
			"name":           []byte("e2e-venus_kibana-sample_892ff7d8-9cf2-48f0-89bc-5a530e77a930"),
			"token":          []byte("AAEAAWVsYXN0aWMva2liYW5hL2RlZmF1bHRfa2liYW5hXzUzYWExOWJiLWM5ZTAtNDhiYS05MTU1LWEyODQ1YmNlZDdmNDpRTzNEVFhuSlZhQ0lnR1FZNjJ0QWRsMEZWSU1FQWhYV1hKcjYxdUdDRGFQMUh4YTNPa3BNSXdOSHkzUTFJbVN2"),
			"hash":           []byte("{PBKDF2_STRETCH}10000$p2YH/lyhXWlOgCbaiPGrArfChZYADB06Aoh9wAbuoPY=$AHRvi+YQK0TZ4kzvWhLiL5+Z1L+UQTVmz7PXndtSou0="),
		},
	}

	expectedElasticsearchUserSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "e2e-mercury",
			Name:            "e2e-venus-kibana-sample-kibana-user",
			UID:             types.UID("0c60f0f4-5847-48b7-b539-c76746a7394f"),
			ResourceVersion: "3443557",
			Labels: map[string]string{
				"common.k8s.elastic.co/type":                     "service-account-token",
				"elasticsearch.k8s.elastic.co/cluster-name":      "elasticsearch-sample",
				"elasticsearch.k8s.elastic.co/cluster-namespace": "e2e-mercury",
				"kibanaassociation.k8s.elastic.co/name":          "kibana-sample",
				"kibanaassociation.k8s.elastic.co/namespace":     "e2e-venus",
				"kibanaassociation.k8s.elastic.co/type":          "elasticsearch",
			},
		},
		Data: map[string][]byte{
			"name": []byte("elastic/kibana/e2e-venus_kibana-sample_892ff7d8-9cf2-48f0-89bc-5a530e77a930"),
			"hash": []byte("{PBKDF2_STRETCH}10000$p2YH/lyhXWlOgCbaiPGrArfChZYADB06Aoh9wAbuoPY=$AHRvi+YQK0TZ4kzvWhLiL5+Z1L+UQTVmz7PXndtSou0="),
		},
	}
)

func Test_ReconcileServiceAccounts(t *testing.T) {
	type args struct {
		client         k8s.Client
		applicationUID types.UID
		serviceAccount commonv1.ServiceAccountName
	}
	tests := []struct {
		name                                 string
		args                                 args
		wantNewToken                         bool
		wantKibanaUserResourceVersion        string
		wantElasticsearchUserResourceVersion string
		wantErr                              bool
	}{
		{
			name: "both secrets do not exist",
			args: args{
				client:         k8s.NewFakeClient(existingElasticsearch.DeepCopy(), existingKibana.DeepCopy()),
				applicationUID: existingKibana.UID,
				serviceAccount: "kibana",
			},
			wantNewToken:                         true,
			wantKibanaUserResourceVersion:        "1", // new Secret
			wantElasticsearchUserResourceVersion: "1", // new Secret
		},
		{
			name: "both secrets already exist, do not update",
			args: args{
				client: k8s.NewFakeClient(
					existingElasticsearch.DeepCopy(),
					existingKibana.DeepCopy(),
					expectedKibanaUserSecret.DeepCopy(),
					expectedElasticsearchUserSecret.DeepCopy(),
				),
				// Kibana resource UID
				applicationUID: existingKibana.UID,
				serviceAccount: "kibana",
			},
			wantKibanaUserResourceVersion:        "3442951", // not updated
			wantElasticsearchUserResourceVersion: "3443557", // not updated
		},
		{
			name: "Elasticsearch secret has been deleted",
			args: args{
				client: k8s.NewFakeClient(
					existingElasticsearch.DeepCopy(),
					existingKibana.DeepCopy(),
					expectedKibanaUserSecret.DeepCopy(),
				),
				// Kibana resource UID
				applicationUID: existingKibana.UID,
				serviceAccount: "kibana",
			},
			wantKibanaUserResourceVersion:        "3442951", // not updated
			wantElasticsearchUserResourceVersion: "1",       // new Secret
		},
		{
			name: "Kibana secret should be recreated",
			args: args{
				client: k8s.NewFakeClient(
					existingElasticsearch.DeepCopy(),
					existingKibana.DeepCopy(),
					expectedElasticsearchUserSecret.DeepCopy(),
				),
				// Kibana resource UID
				applicationUID: existingKibana.UID,
				serviceAccount: "kibana",
			},
			wantNewToken:                         true,
			wantKibanaUserResourceVersion:        "1",       // new Secret
			wantElasticsearchUserResourceVersion: "3443558", // updated with new token
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commonLabels := map[string]string{
				"elasticsearch.k8s.elastic.co/cluster-name":      "elasticsearch-sample",
				"elasticsearch.k8s.elastic.co/cluster-namespace": "e2e-mercury",
				"kibanaassociation.k8s.elastic.co/type":          "elasticsearch",
				"kibanaassociation.k8s.elastic.co/name":          "kibana-sample",
				"kibanaassociation.k8s.elastic.co/namespace":     "e2e-venus",
			}
			applicationSecretName := types.NamespacedName{Namespace: "e2e-venus", Name: "kibana-sample-kibana-user"}
			elasticsearchSecretName := types.NamespacedName{Namespace: "e2e-mercury", Name: "e2e-venus-kibana-sample-kibana-user"}
			err := ReconcileServiceAccounts(
				context.Background(),
				tt.args.client,
				existingElasticsearch,
				commonLabels,
				applicationSecretName,
				elasticsearchSecretName,
				tt.args.serviceAccount,
				existingKibana.Name,
				existingKibana.UID,
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("store.EnsureTokenExists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check Secrets once ReconcileServiceAccounts has been called
			reconciledKibanaSecret := corev1.Secret{}
			assert.NoError(t, tt.args.client.Get(context.Background(), applicationSecretName, &reconciledKibanaSecret))
			assert.Equal(t, expectedKibanaUserSecret.Labels, reconciledKibanaSecret.Labels, "Labels on application SA secret are not equal")
			assert.Equal(t, tt.wantKibanaUserResourceVersion, reconciledKibanaSecret.ResourceVersion, "Unexpected Kibana Secret resource version")

			reconciledElasticsearchSecret := corev1.Secret{}
			assert.NoError(t, tt.args.client.Get(context.Background(), elasticsearchSecretName, &reconciledElasticsearchSecret))
			assert.Equal(t, expectedElasticsearchUserSecret.Labels, reconciledElasticsearchSecret.Labels, "Labels on Elasticsearch SA secret are not equal")
			assert.Equal(t, tt.wantElasticsearchUserResourceVersion, reconciledElasticsearchSecret.ResourceVersion, "Unexpected Elasticsearch Secret resource version")

			assert.Equal(t, 4, len(reconciledKibanaSecret.Data))

			serviceAccountName, exist := reconciledKibanaSecret.Data["serviceAccount"]
			assert.True(t, exist)
			assert.Equal(t, "elastic/kibana", string(serviceAccountName))

			name, exist := reconciledKibanaSecret.Data["name"]
			assert.True(t, exist)
			assert.Equal(t, "e2e-venus_kibana-sample_892ff7d8-9cf2-48f0-89bc-5a530e77a930", string(name))

			hash, exist := reconciledKibanaSecret.Data["hash"]
			assert.True(t, exist)
			token, exist := reconciledKibanaSecret.Data["token"]
			assert.True(t, exist)

			assert.Equal(t, 2, len(reconciledElasticsearchSecret.Data))
			esName, exist := reconciledElasticsearchSecret.Data["name"]
			assert.True(t, exist)
			assert.Equal(t, "elastic/kibana/e2e-venus_kibana-sample_892ff7d8-9cf2-48f0-89bc-5a530e77a930", string(esName))

			esHash, exist := reconciledElasticsearchSecret.Data["hash"]
			assert.True(t, exist)
			assert.Equal(t, hash, esHash)

			if tt.wantNewToken {
				// Only check that the expected fields are there and that the generated token is valid
				verifyToken(t, string(token), string(hash), "kibana", string(name))
				// Previous hash and token should not be there anymore
				hash, exist := reconciledKibanaSecret.Data["hash"]
				assert.True(t, exist)
				assert.NotEqual(t, "{PBKDF2_STRETCH}10000$p2YH/lyhXWlOgCbaiPGrArfChZYADB06Aoh9wAbuoPY=$AHRvi+YQK0TZ4kzvWhLiL5+Z1L+UQTVmz7PXndtSou0=", string(hash))
				token, exist := reconciledKibanaSecret.Data["token"]
				assert.True(t, exist)
				assert.NotEqual(t, "AAEAAWVsYXN0aWMva2liYW5hL2RlZmF1bHRfa2liYW5hXzUzYWExOWJiLWM5ZTAtNDhiYS05MTU1LWEyODQ1YmNlZDdmNDpRTzNEVFhuSlZhQ0lnR1FZNjJ0QWRsMEZWSU1FQWhYV1hKcjYxdUdDRGFQMUh4YTNPa3BNSXdOSHkzUTFJbVN2", string(token))
			} else {
				// Reuse existing hash and token
				hash, exist := reconciledKibanaSecret.Data["hash"]
				assert.True(t, exist)
				assert.Equal(t, "{PBKDF2_STRETCH}10000$p2YH/lyhXWlOgCbaiPGrArfChZYADB06Aoh9wAbuoPY=$AHRvi+YQK0TZ4kzvWhLiL5+Z1L+UQTVmz7PXndtSou0=", string(hash))
				token, exist := reconciledKibanaSecret.Data["token"]
				assert.True(t, exist)
				assert.Equal(t, "AAEAAWVsYXN0aWMva2liYW5hL2RlZmF1bHRfa2liYW5hXzUzYWExOWJiLWM5ZTAtNDhiYS05MTU1LWEyODQ1YmNlZDdmNDpRTzNEVFhuSlZhQ0lnR1FZNjJ0QWRsMEZWSU1FQWhYV1hKcjYxdUdDRGFQMUh4YTNPa3BNSXdOSHkzUTFJbVN2", string(token))
			}
		})
	}
}

func Test_newApplicationToken(t *testing.T) {
	type args struct {
		serviceAccountName commonv1.ServiceAccountName
		tokenName          string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Simple test with Key validation",
			args: args{
				serviceAccountName: kbv1.KibanaServiceAccount,
				tokenName:          "e2e-venus-kibana-kibana-sample",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newApplicationToken(tt.args.serviceAccountName, tt.args.tokenName)
			if (err != nil) != tt.wantErr {
				t.Errorf("newApplicationToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			verifyToken(t, got.Token, got.Hash, string(tt.args.serviceAccountName), tt.args.tokenName)
		})
	}
}

var (
	secretRegEx     = regexp.MustCompile(`^([-\w]+)\/([-\w]+)\/([-\w]+)\:(\w+)$`)
	pbkdf2HashRegEx = regexp.MustCompile(`^\{PBKDF2_STRETCH\}10000\$(.*)\$(.*)$`)
)

func verifyToken(t *testing.T, b64token, hash, serviceAccount, tokenName string) {
	t.Helper()

	// Validate the hash
	sub := pbkdf2HashRegEx.FindStringSubmatch(hash)
	assert.Equal(t, 3, len(sub), "PBKDF2 hash does not match regexp %s", pbkdf2HashRegEx)
	salt, err := base64.StdEncoding.DecodeString(sub[1])
	assert.NoError(t, err, "Unexpected error while decoding salt")
	assert.Equal(t, len(salt), pbkdf2DefaultSaltLength)
	hashString, err := base64.StdEncoding.DecodeString(sub[2])
	assert.NoError(t, err, "Unexpected error while decoding hash string")
	assert.True(t, len(hashString) > 0, "Hash string should not be empty")

	// Validate the token
	token, err := base64.StdEncoding.DecodeString(b64token)
	assert.NoError(t, err, "Unexpected error while decoding token")
	// Check token prefix
	assert.Equal(t, byte(0x00), token[0])
	assert.Equal(t, byte(0x01), token[1])
	assert.Equal(t, byte(0x00), token[2])
	assert.Equal(t, byte(0x01), token[3])
	// Decode and check token suffix
	secretSuffix := string(token[4:])
	assert.True(t, len(secretSuffix) > 0, "Secret body should not be empty")
	tokenSuffix := secretRegEx.FindStringSubmatch(secretSuffix)
	assert.Equal(t, 5, len(tokenSuffix), "Secret suffix does not match regexp")
	assert.Equal(t, Namespace, tokenSuffix[1])
	assert.Equal(t, serviceAccount, tokenSuffix[2])
	assert.Equal(t, tokenName, tokenSuffix[3])
	clearTextTokenSecret := tokenSuffix[4]

	verify(t, []byte(clearTextTokenSecret), hash)
}

func Test_hash(t *testing.T) {
	type args struct {
		secret []byte
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Simple hash test",
			args: args{
				secret: []byte("asecret"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pbkdf2Key(tt.args.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("hash() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				assert.Nil(t, got)
			} else {
				assert.NotNil(t, got)
				verify(t, tt.args.secret, got)
			}
		})
	}
}

func verify(t *testing.T, password []byte, hash string) {
	t.Helper()

	// password is hashed using sha512 and converted to a hex string
	hashedSecret := sha512.Sum512(password)
	hashedSecretAsString := hex.EncodeToString(hashedSecret[:])

	// Base64 string length : (4*(n/3)) rounded up to the next multiple of 4 because of padding.
	// n is 32 (PBKDF2_KEY_LENGTH in bytes), so tokenLength is 44
	tokenLength := 44
	hashChars := hash[len(hash)-tokenLength:]
	saltChars := hash[len(hash)-(2*tokenLength+1) : len(hash)-(tokenLength+1)]
	salt, err := base64.StdEncoding.DecodeString(saltChars)
	assert.NoError(t, err)

	costChars := hash[len(pbkdf2StretchPrefix) : len(hash)-(2*tokenLength+2)]
	cost, err := strconv.Atoi(costChars)
	assert.NoError(t, err)

	dk := pbkdf2.Key([]byte(hashedSecretAsString), salt, cost, pbkdf2KeyLength, sha512.New)
	computedPwdHash := base64.StdEncoding.EncodeToString(dk)

	assert.Equal(t, hashChars, computedPwdHash)
}

func Test_TokenMarshalJSON(t *testing.T) {
	tests := []struct {
		name  string
		token Token
		want  string
	}{
		{
			name: "Secret data should not be serialized",
			token: Token{
				ServiceAccountName: "service_account",
				TokenName:          "token_name",
				Token:              "secret",
				Hash:               "secret",
			},
			want: `{"serviceAccountName":"service_account","tokenName":"token_name","token":"REDACTED","hash":"REDACTED"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ser, err := json.Marshal(tt.token)
			assert.Nil(t, err)
			assert.Equal(t, string(ser), tt.want)
		})
	}
}
