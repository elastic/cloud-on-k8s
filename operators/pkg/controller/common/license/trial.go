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

	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/chrono"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
)

const (
	TrialStatusSecretKey = "trial-status"
	TrialPubkeyKey       = "pubkey"
)

func InitTrial(c k8s.Client, secret corev1.Secret, l *EnterpriseLicense) (*rsa.PublicKey, error) {
	if l == nil {
		return nil, errors.New("license is nil")
	}

	if err := populateTrialLicense(l); err != nil {
		return nil, pkgerrors.Wrap(err, "failed to populate trial license")
	}
	log.Info("Starting enterprise trial", "start", l.StartTime(), "end", l.ExpiryTime())
	rnd := rand.Reader
	tmpPrivKey, err := rsa.GenerateKey(rnd, 2048)
	if err != nil {
		return nil, err
	}
	// sign trial license
	signer := NewSigner(tmpPrivKey)
	sig, err := signer.Sign(*l)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "failed to sign license")
	}
	pubkeyBytes, err := x509.MarshalPKIXPublicKey(&tmpPrivKey.PublicKey)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "failed to marshal public key for trial status")
	}
	trialStatus := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secret.Namespace,
			Name:      TrialStatusSecretKey,
			Labels: map[string]string{
				LicenseLabelName: l.License.UID,
			},
		},
		Data: map[string][]byte{
			TrialPubkeyKey: pubkeyBytes,
		},
	}
	err = c.Create(&trialStatus)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "failed to create trial status")
	}
	l.License.Signature = string(sig)
	// return pub key to retain in memory for later iterations
	return &tmpPrivKey.PublicKey, pkgerrors.Wrap(
		UpdateEnterpriseLicense(c, secret, *l),
		"failed to update trial license",
	)
}

// populateTrialLicense adds missing fields to a trial license.
func populateTrialLicense(l *EnterpriseLicense) error {
	if !l.IsTrial() {
		return fmt.Errorf("%s for %s is not a trial license", l.License.UID, l.License.IssuedTo)
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
