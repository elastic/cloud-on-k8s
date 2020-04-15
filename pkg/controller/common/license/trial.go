// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/utils/chrono"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
)

const (
	TrialStatusSecretKey = "trial-status"
	TrialPubkeyKey       = "pubkey"
	TrialPrivateKey      = "key"

	TrialLicenseSecretName      = "trial.k8s.elastic.co/secret-name"      // nolint
	TrialLicenseSecretNamespace = "trial.k8s.elastic.co/secret-namespace" // nolint
)

func ExpectedTrialStatusWithPK(operatorNamespace string, license types.NamespacedName, key *rsa.PrivateKey) (corev1.Secret, error) {
	status, err := ExpectedTrialStatus(operatorNamespace, license, &key.PublicKey)
	if err != nil {
		return status, err
	}

	// handle a combination of operator crashes and API errors on trial activation by keeping this around
	status.Data[TrialPrivateKey] = x509.MarshalPKCS1PrivateKey(key)
	return status, nil
}

func ExpectedTrialStatus(operatorNamespace string, license types.NamespacedName, key *rsa.PublicKey) (corev1.Secret, error) {
	pubkeyBytes, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return corev1.Secret{}, err
	}
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: operatorNamespace,
			Name:      TrialStatusSecretKey,
			Annotations: map[string]string{
				TrialLicenseSecretName:      license.Name,
				TrialLicenseSecretNamespace: license.Namespace,
			},
		},
		Data: map[string][]byte{
			TrialPubkeyKey: pubkeyBytes,
		},
	}, nil
}

func NewTrialKey() (*rsa.PrivateKey, error) {
	rnd := rand.Reader
	trialKey, err := rsa.GenerateKey(rnd, 2048)
	if err != nil {
		return nil, fmt.Errorf("while generating trial key %w", err)
	}
	return trialKey, nil
}

func InitTrial(key *rsa.PrivateKey, l *EnterpriseLicense) error {
	if l == nil {
		return errors.New("license is nil")
	}
	if err := populateTrialLicense(l); err != nil {
		return pkgerrors.Wrap(err, "failed to populate trial license")
	}

	log.Info("Starting enterprise trial", "start", l.StartTime(), "end", l.ExpiryTime())
	// sign trial license
	signer := NewSigner(key)
	sig, err := signer.Sign(*l)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to sign license")
	}

	l.License.Signature = string(sig)
	return nil
}

// populateTrialLicense adds missing fields to a trial license.
func populateTrialLicense(l *EnterpriseLicense) error {
	if !l.IsTrial() {
		return pkgerrors.Errorf("%s for %s is not a trial license", l.License.UID, l.License.IssuedTo)
	}
	if l.License.Issuer == "" {
		l.License.Issuer = "Elastic k8s operator"
	}
	if l.License.IssuedTo == "" {
		l.License.IssuedTo = "Unknown"
	}
	if l.License.UID == "" {
		l.License.UID = string(uuid.NewUUID())
	}

	if l.License.StartDateInMillis == 0 || l.License.ExpiryDateInMillis == 0 {
		setStartAndExpiry(l, time.Now())
	}
	return nil
}

// setStartAndExpiry sets the issue, start and end dates for a trial.
func setStartAndExpiry(l *EnterpriseLicense, from time.Time) {
	l.License.StartDateInMillis = chrono.ToMillis(from)
	l.License.IssueDateInMillis = l.License.StartDateInMillis
	l.License.ExpiryDateInMillis = chrono.ToMillis(from.Add(24 * time.Hour * 30))
}
