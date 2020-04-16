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
	"strconv"
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
	TrialActivationKey   = "in-trial-activation"

	TrialLicenseSecretName      = "trial.k8s.elastic.co/secret-name"      // nolint
	TrialLicenseSecretNamespace = "trial.k8s.elastic.co/secret-namespace" // nolint
)

// TrialState capture the in-memory representation of the trial status.
type TrialState struct {
	privateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
}

// NewTrialState creates a set of trial keys by generating a new RSA key pair.
func NewTrialState() (TrialState, error) {
	key, err := NewTrialKey()
	if err != nil {
		return TrialState{}, err
	}
	return TrialState{
		privateKey: key,
		PublicKey:  &key.PublicKey,
	}, nil
}

// NewTrialStateFromStatus reconstructs trial keys from a trial status secret.
func NewTrialStateFromStatus(trialStatus corev1.Secret) (TrialState, error) {
	// reinstate pubkey from status secret e.g. after operator restart
	pubKeyBytes := trialStatus.Data[TrialPubkeyKey]
	key, err := ParsePubKey(pubKeyBytes)
	if err != nil {
		return TrialState{}, err
	}
	keys := TrialState{
		PublicKey: key,
	}
	// create new keys if the operator failed just before the trial was started
	trialActivation, err := strconv.ParseBool(string(trialStatus.Data[TrialActivationKey]))
	if err == nil && trialActivation {
		return NewTrialState()
	}
	return keys, nil
}

// IsTrialRunning returns true if a trial has been successfully started at some point in the past.
func (tk *TrialState) IsTrialRunning() bool {
	return tk.PublicKey != nil && tk.privateKey == nil
}

// IsTrialActivtationInProgress returns true if we are in the process of starting a trial.
func (tk *TrialState) IsTrialActivationInProgress() bool {
	return tk.privateKey != nil && tk.PublicKey != nil
}

func (tk *TrialState) InitTrialLicense(l *EnterpriseLicense) error {
	if !tk.IsTrialActivationInProgress() {
		return errors.New("trial has already been activated")
	}
	if l == nil {
		return errors.New("license is nil")
	}
	if err := populateTrialLicense(l); err != nil {
		return pkgerrors.Wrap(err, "failed to populate trial license")
	}

	log.Info("Starting enterprise trial", "start", l.StartTime(), "end", l.ExpiryTime())
	// sign trial license
	signer := NewSigner(tk.privateKey)
	sig, err := signer.Sign(*l)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to sign license")
	}

	l.License.Signature = string(sig)
	return nil
}

func (tk *TrialState) CompleteTrialActivation() bool {
	if tk.privateKey == nil {
		return false
	}
	tk.privateKey = nil
	return true
}

// ExpectedTrialStatus creates the expected state of the trial status secret for the given keys for reconciliation.
func ExpectedTrialStatus(operatorNamespace string, license types.NamespacedName, state TrialState) (corev1.Secret, error) {
	pubkeyBytes, err := x509.MarshalPKIXPublicKey(state.PublicKey)
	if err != nil {
		return corev1.Secret{}, err
	}
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: operatorNamespace,
			Name:      TrialStatusSecretKey,
			Annotations: map[string]string{
				TrialLicenseSecretName:      license.Name,
				TrialLicenseSecretNamespace: license.Namespace,
			},
		},
		Data: map[string][]byte{
			TrialPubkeyKey:     pubkeyBytes,
			TrialActivationKey: []byte(strconv.FormatBool(state.IsTrialActivationInProgress())),
		},
	}
	return secret, nil
}

func NewTrialKey() (*rsa.PrivateKey, error) {
	rnd := rand.Reader
	trialKey, err := rsa.GenerateKey(rnd, 2048)
	if err != nil {
		return nil, fmt.Errorf("while generating trial key %w", err)
	}
	return trialKey, nil
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
