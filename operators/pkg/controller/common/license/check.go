// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Checker contains parameters for license checks.
type Checker struct {
	k8sClient         k8s.Client
	operatorNamespace string
	publicKey         []byte
}

// NewLicenseChecker creates a new license checker.
func NewLicenseChecker(client k8s.Client, operatorNamespace string) *Checker {
	return &Checker{
		k8sClient:         client,
		operatorNamespace: operatorNamespace,
		publicKey:         publicKeyBytes,
	}
}

// EnterpriseFeaturesEnabled returns true if a valid enterprise license is installed.
func (lc *Checker) EnterpriseFeaturesEnabled() (bool, error) {
	licenses, err := EnterpriseLicenseList(lc.k8sClient)
	if err != nil {
		return false, errors.Wrap(err, "failed to list enterprise licenses")
	}

	for _, l := range licenses {
		pk := lc.publicKey
		if l.IsTrial() {
			var signatureSec corev1.Secret
			err := lc.k8sClient.Get(types.NamespacedName{
				Namespace: lc.operatorNamespace,
				Name:      TrialStatusSecretKey,
			}, &signatureSec)
			if err != nil {
				return false, errors.Wrap(err, "while loading signature secret")
			}
			pk = signatureSec.Data[TrialPubkeyKey]
		}
		verifier, err := NewVerifier(pk)
		if err != nil {
			log.Error(err, "while creating license verifier")
			continue
		}
		status := verifier.Valid(l, time.Now())
		if status == v1alpha1.LicenseStatusValid {
			return true, nil
		}
	}
	return false, nil
}
