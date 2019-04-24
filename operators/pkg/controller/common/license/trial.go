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

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/chrono"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
)

const (
	TrialStatusSecretKey = "trial-status"
	TrialPubkeyKey       = "pubkey"
	TrialSignatureKey    = "signature"
	TrialFinalizerName   = "trial/finalizers.k8s.elastic.co" // slash required on core object finalizers to be fully qualified
)

func InitTrial(c k8s.Client, l *estype.EnterpriseLicense) (*rsa.PublicKey, error) {
	if l == nil {
		return nil, errors.New("license is nil")
	}

	if err := populateTrialLicense(l); err != nil {
		return nil, pkgerrors.Wrap(err, "Failed to populate trial license")
	}
	log.Info("Starting enterprise trial", "start", l.StartDate(), "end", l.ExpiryDate())
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
			Namespace: l.Namespace,
			Name:      TrialStatusSecretKey,
			Labels: map[string]string{
				EnterpriseLicenseLabelName: l.Name,
			},
			Finalizers: []string{
				TrialFinalizerName,
			},
		},
		Data: map[string][]byte{
			TrialSignatureKey: sig,
			TrialPubkeyKey:    pubkeyBytes,
		},
	}
	err = c.Create(&trialStatus)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "Failed to create trial status")
	}
	l.Finalizers = append(l.Finalizers, TrialFinalizerName)
	l.Spec.SignatureRef = corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: TrialStatusSecretKey,
		},
		Key: TrialSignatureKey,
	}
	l.Status = estype.LicenseStatusValid
	// return pub key to retain in memory for later iterations
	return &tmpPrivKey.PublicKey, pkgerrors.Wrap(c.Update(l), "Failed to update trial license")
}

// populateTrialLicense adds missing fields to a trial license.
func populateTrialLicense(l *estype.EnterpriseLicense) error {
	if !l.IsTrial() {
		return fmt.Errorf("%v is not a trial license", k8s.ExtractNamespacedName(l))
	}
	if err := l.IsMissingFields(); err != nil {
		l.Spec.Issuer = "Elastic k8s operator"
		l.Spec.IssuedTo = "Unknown"
		l.Spec.UID = string(uuid.NewUUID())
		setStartAndExpiry(l, time.Now())
	}
	return nil

}

// setStartAndExpiry sets the issue, start and end dates for a trial.
func setStartAndExpiry(l *v1alpha1.EnterpriseLicense, from time.Time) {
	l.Spec.StartDateInMillis = chrono.ToMillis(from)
	l.Spec.IssueDateInMillis = l.Spec.StartDateInMillis
	l.Spec.ExpiryDateInMillis = chrono.ToMillis(from.Add(24 * time.Hour * 30))
}
