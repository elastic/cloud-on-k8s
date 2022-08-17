// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/chrono"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	ECKLicenseIssuer = "Elastic k8s operator"

	TrialStatusSecretKey = "trial-status"
	TrialPubkeyKey       = "pubkey"
	TrialActivationKey   = "in-trial-activation"

	TrialLicenseSecretName      = "trial.k8s.elastic.co/secret-name"      //nolint:gosec
	TrialLicenseSecretNamespace = "trial.k8s.elastic.co/secret-namespace" //nolint:gosec
)

// TrialState captures the in-memory representation of the trial status.
type TrialState struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
}

// NewTrialState creates a new trial state based on a new RSA key pair.
func NewTrialState() (TrialState, error) {
	key, err := newTrialKey()
	if err != nil {
		return TrialState{}, err
	}
	return TrialState{
		privateKey: key,
		publicKey:  &key.PublicKey,
	}, nil
}

// NewTrialStateFromStatus reconstructs trial state from a trial status secret.
func NewTrialStateFromStatus(trialStatus corev1.Secret) (TrialState, error) {
	// reinstate pubkey from status secret e.g. after operator restart
	pubKeyBytes := trialStatus.Data[TrialPubkeyKey]
	key, err := ParsePubKey(pubKeyBytes)
	if err != nil {
		return TrialState{}, err
	}
	return TrialState{
		publicKey: key,
	}, nil
}

// IsTrialStarted returns true if a trial has been successfully started at some point in the past.
func (tk *TrialState) IsTrialStarted() bool {
	return tk.publicKey != nil && tk.privateKey == nil
}

// IsEmpty returns true on an empty state struct.
func (tk *TrialState) IsEmpty() bool {
	return tk.privateKey == nil && tk.publicKey == nil
}

// InitTrialLicense initialises and signs the given license based on the current state.
func (tk *TrialState) InitTrialLicense(ctx context.Context, l *EnterpriseLicense) error {
	if tk.privateKey == nil {
		return errors.New("trial has already been activated")
	}
	if l == nil {
		return errors.New("license is nil")
	}
	if err := populateTrialLicense(l); err != nil {
		return pkgerrors.Wrap(err, "failed to populate trial license")
	}

	ulog.FromContext(ctx).Info("Starting enterprise trial", "start", l.StartTime(), "end", l.ExpiryTime())
	// sign trial license
	signer := NewSigner(tk.privateKey)
	sig, err := signer.Sign(*l)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to sign license")
	}

	l.License.Signature = string(sig)
	return nil
}

// CompleteTrialActivation should be called once a trial license has been successfully generated and verified.
// Returns false if the trial activation had been completed previously.
func (tk *TrialState) CompleteTrialActivation() bool {
	if tk.privateKey == nil {
		return false
	}
	tk.privateKey = nil
	return true
}

// LicenseVerifier returns a verifier based on the current state/public key
func (tk *TrialState) LicenseVerifier() *Verifier {
	return &Verifier{PublicKey: tk.publicKey}
}

// ExpectedTrialStatus creates the expected state of the trial status secret for the given trial state for reconciliation purposes.
func ExpectedTrialStatus(operatorNamespace string, license types.NamespacedName, state TrialState) (corev1.Secret, error) {
	if state.IsEmpty() {
		return corev1.Secret{}, errors.New("cannot create trial status from uninitialised trial state")
	}
	pubkeyBytes, err := x509.MarshalPKIXPublicKey(state.publicKey)
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
			TrialPubkeyKey: pubkeyBytes,
		},
	}
	if !state.IsTrialStarted() {
		secret.Data[TrialActivationKey] = []byte("true")
	}
	return secret, nil
}

func newTrialKey() (*rsa.PrivateKey, error) {
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
		l.License.Issuer = ECKLicenseIssuer
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
