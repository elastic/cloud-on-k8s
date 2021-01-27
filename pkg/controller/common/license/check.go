// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"context"
	"sort"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type Checker interface {
	CurrentEnterpriseLicense() (*EnterpriseLicense, error)
	EnterpriseFeaturesEnabled() (bool, error)
	Valid(l EnterpriseLicense) (bool, error)
}

// checker contains parameters for license checks.
type checker struct {
	k8sClient         k8s.Client
	operatorNamespace string
	publicKey         []byte
}

// NewLicenseChecker creates a new license checker.
func NewLicenseChecker(client k8s.Client, operatorNamespace string) Checker {
	return &checker{
		k8sClient:         client,
		operatorNamespace: operatorNamespace,
		publicKey:         publicKeyBytes,
	}
}

func (lc *checker) publicKeyFor(l EnterpriseLicense) ([]byte, error) {
	if !l.IsECKManagedTrial() {
		return lc.publicKey, nil
	}

	var signatureSec corev1.Secret
	err := lc.k8sClient.Get(context.Background(), types.NamespacedName{
		Namespace: lc.operatorNamespace,
		Name:      TrialStatusSecretKey,
	}, &signatureSec)
	// read secret data only after retrieving the secret
	return signatureSec.Data[TrialPubkeyKey], err
}

// CurrentEnterpriseLicense returns the currently valid Enterprise license if installed.
func (lc *checker) CurrentEnterpriseLicense() (*EnterpriseLicense, error) {
	licenses, err := EnterpriseLicenses(lc.k8sClient)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list enterprise licenses")
	}

	sort.Slice(licenses, func(i, j int) bool {
		t1, t2 := EnterpriseLicenseTypeOrder[licenses[i].License.Type], EnterpriseLicenseTypeOrder[licenses[j].License.Type]
		if t1 != t2 { // sort by type (first the most features)
			return t1 > t2
		}
		// and by expiry date (first which expires last)
		return licenses[i].License.ExpiryDateInMillis > licenses[j].License.ExpiryDateInMillis
	})

	// pick the first valid Enterprise license in the sorted slice
	for _, l := range licenses {
		valid, err := lc.Valid(l)
		if err != nil {
			return nil, err
		}
		if valid {
			return &l, nil
		}
	}
	return nil, nil
}

// EnterpriseFeaturesEnabled returns true if a valid enterprise license is installed.
func (lc *checker) EnterpriseFeaturesEnabled() (bool, error) {
	license, err := lc.CurrentEnterpriseLicense()
	if err != nil {
		return false, err
	}
	return license != nil, nil
}

// Valid returns true if the given Enterprise license is valid or an error if any.
func (lc *checker) Valid(l EnterpriseLicense) (bool, error) {
	pk, err := lc.publicKeyFor(l)
	if err != nil {
		return false, errors.Wrap(err, "while loading signature secret")
	}
	if len(pk) == 0 {
		log.Info("This is an unlicensed development build of ECK. License management and Enterprise features are disabled")
		return false, nil
	}
	verifier, err := NewVerifier(pk)
	if err != nil {
		return false, err
	}
	status := verifier.Valid(l, time.Now())
	if status == LicenseStatusValid {
		return true, nil
	}
	return false, nil
}

type MockChecker struct{}

func (MockChecker) CurrentEnterpriseLicense() (*EnterpriseLicense, error) {
	return &EnterpriseLicense{}, nil
}

func (MockChecker) EnterpriseFeaturesEnabled() (bool, error) {
	return true, nil
}

func (MockChecker) Valid(l EnterpriseLicense) (bool, error) {
	return true, nil
}

var _ Checker = MockChecker{}
