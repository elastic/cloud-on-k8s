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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
)

const (
	TrialStatusSecretKey = "trial-status"
	TrialPubkeyKey       = "pubkey"
)

func InitTrial(c k8s.Client, namespace string, l *SourceEnterpriseLicense) (*rsa.PublicKey, error) {
	if l == nil {
		return nil, errors.New("license is nil")
	}

	if err := populateTrialLicense(l); err != nil {
		return nil, pkgerrors.Wrap(err, "Failed to populate trial license")
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
		return nil, pkgerrors.Wrap(err, "Failed to sign license")
	}
	pubkeyBytes, err := x509.MarshalPKIXPublicKey(&tmpPrivKey.PublicKey)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "Failed to marshal public key for trial status")
	}
	trialStatus := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      TrialStatusSecretKey,
			Labels: map[string]string{
				LicenseLabelName: l.Data.UID,
			},
		},
		Data: map[string][]byte{
			TrialPubkeyKey: pubkeyBytes,
		},
	}
	err = c.Create(&trialStatus)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "Failed to create trial status")
	}
	l.Data.Signature = string(sig)
	// return pub key to retain in memory for later iterations
	return &tmpPrivKey.PublicKey, pkgerrors.Wrap(
		CreateEnterpriseLicense(
			c,
			types.NamespacedName{
				Namespace: namespace,
				Name:      l.Data.UID,
			},
			*l,
		),
		"Failed to update trial license",
	)
}

// populateTrialLicense adds missing fields to a trial license.
func populateTrialLicense(l *SourceEnterpriseLicense) error {
	if !l.IsTrial() {
		return fmt.Errorf("%s for %s is not a trial license", l.Data.UID, l.Data.IssuedTo)
	}
	if l.Data.Issuer == "" {
		l.Data.Issuer = "Elastic k8s operator"
	}
	if l.Data.IssuedTo == "" {
		l.Data.IssuedTo = "Unknown"
	}
	if l.Data.UID == "" {
		l.Data.UID = string(uuid.NewUUID())
	}

	if l.Data.StartDateInMillis == 0 || l.Data.ExpiryDateInMillis == 0 {
		setStartAndExpiry(l, time.Now())
	}
	return nil
}

// setStartAndExpiry sets the issue, start and end dates for a trial.
func setStartAndExpiry(l *SourceEnterpriseLicense, from time.Time) {
	l.Data.StartDateInMillis = chrono.ToMillis(from)
	l.Data.IssueDateInMillis = l.Data.StartDateInMillis
	l.Data.ExpiryDateInMillis = chrono.ToMillis(from.Add(24 * time.Hour * 30))
}
