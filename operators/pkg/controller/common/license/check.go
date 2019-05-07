// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	var licenses v1alpha1.EnterpriseLicenseList
	err := lc.k8sClient.List(&client.ListOptions{}, &licenses)
	if err != nil {
		return false, errors.Wrap(err, "while reading licenses")
	}

	for _, l := range licenses.Items {
		sigRef := l.Spec.SignatureRef
		var signatureSec corev1.Secret

		err := lc.k8sClient.Get(types.NamespacedName{
			Namespace: lc.operatorNamespace,
			Name:      sigRef.Name,
		}, &signatureSec)
		if err != nil {
			return false, errors.Wrap(err, "while loading signature secret")
		}

		pk := lc.publicKey
		if l.Spec.Type == v1alpha1.LicenseTypeEnterpriseTrial {
			pk = signatureSec.Data[TrialPubkeyKey]
		}
		verifier, err := NewVerifier(pk)
		if err != nil {
			log.Error(err, "while creating license verifier")
			continue
		}
		status := verifier.Valid(l, signatureSec.Data[sigRef.Key], time.Now())
		if status == v1alpha1.LicenseStatusValid {
			return true, nil
		}
	}
	return false, nil
}
