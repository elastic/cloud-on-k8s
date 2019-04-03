/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package mutation

import (
	"fmt"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/pkg/errors"
)

func toMillis(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond)
}

// PopulateTrialLicense adds missing fields to a trial license.
func PopulateTrialLicense(l *v1alpha1.EnterpriseLicense) error {
	if l == nil {
		return errors.New("license is nil")
	}
	if !l.IsTrial() {
		return fmt.Errorf("%v is not a trial license", k8s.ExtractNamespacedName(l))
	}
	if err := l.IsMissingFields(); err != nil {
		l.Spec.Issuer = "Elastic k8s operator"
		l.Spec.IssuedTo = "Unknown"
		l.Spec.UID = string(uuid.NewUUID())
		StartTrial(l, time.Now()) // pre-populating these here for completeness will be overridden on actual trial start
	}
	return nil

}

// StartTrial sets the issue, start and end dates for a trial.
func StartTrial(l *v1alpha1.EnterpriseLicense, from time.Time) {
	l.Spec.StartDateInMillis = toMillis(from)
	l.Spec.IssueDateInMillis = l.Spec.StartDateInMillis
	l.Spec.ExpiryDateInMillis = toMillis(from.Add(24 * time.Hour * 30))
}
