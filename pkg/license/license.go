// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/metrics"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// defaultOperatorLicenseLevel is the default license level when no operator license is installed
	defaultOperatorLicenseLevel = "basic"
	// LicensingCfgMapName is the name of the config map used to store licensing information
	LicensingCfgMapName = "elastic-licensing"
	// Type represents the Elastic usage type used to mark the config map that stores licensing information
	Type = "elastic-usage"
)

// LicensingInfo represents information about the operator license including the total memory of all Elastic managed
// components
type LicensingInfo struct {
	Timestamp                  string
	EckLicenseLevel            string
	TotalManagedMemory         float64
	MaxEnterpriseResourceUnits int64
	EnterpriseResourceUnits    int64
}

// toMap transforms a LicensingInfo to a map of string, in order to fill in the data of a config map
func (li LicensingInfo) toMap() map[string]string {
	m := map[string]string{
		"timestamp":                 li.Timestamp,
		"eck_license_level":         li.EckLicenseLevel,
		"total_managed_memory":      fmt.Sprintf("%0.2fGB", li.TotalManagedMemory),
		"enterprise_resource_units": strconv.FormatInt(li.EnterpriseResourceUnits, 10),
	}

	if li.MaxEnterpriseResourceUnits > 0 {
		m["max_enterprise_resource_units"] = strconv.FormatInt(li.MaxEnterpriseResourceUnits, 10)
	}

	return m
}

func (li LicensingInfo) ReportAsMetrics() {
	labels := prometheus.Labels{metrics.LicenseLevelLabel: li.EckLicenseLevel}
	metrics.LicensingTotalMemoryGauge.With(labels).Set(li.TotalManagedMemory)
	metrics.LicensingTotalERUGauge.With(labels).Set(float64(li.EnterpriseResourceUnits))

	if li.MaxEnterpriseResourceUnits > 0 {
		metrics.LicensingMaxERUGauge.With(labels).Set(float64(li.MaxEnterpriseResourceUnits))
	}
}

// LicensingResolver resolves the licensing information of the operator
type LicensingResolver struct {
	operatorNs string
	client     k8s.Client
}

// ToInfo returns licensing information given the total memory of all Elastic managed components
func (r LicensingResolver) ToInfo(totalMemory resource.Quantity) (LicensingInfo, error) {
	operatorLicense, err := r.getOperatorLicense()
	if err != nil {
		return LicensingInfo{}, err
	}

	licensingInfo := LicensingInfo{
		Timestamp:               time.Now().Format(time.RFC3339),
		EckLicenseLevel:         r.getOperatorLicenseLevel(operatorLicense),
		TotalManagedMemory:      inGB(totalMemory),
		EnterpriseResourceUnits: inEnterpriseResourceUnits(totalMemory),
	}

	// include the max ERUs only for a non trial/basic license
	if maxERUs := r.getMaxEnterpriseResourceUnits(operatorLicense); maxERUs > 0 {
		licensingInfo.MaxEnterpriseResourceUnits = maxERUs
	}

	return licensingInfo, nil
}

// Save updates or creates licensing information in a config map
// This relies on UnconditionalUpdates being supported configmaps and may change in k8s v2: https://github.com/kubernetes/kubernetes/issues/21330
func (r LicensingResolver) Save(info LicensingInfo) error {
	log.V(1).Info("Saving", "namespace", r.operatorNs, "configmap_name", LicensingCfgMapName, "license_info", info)
	nsn := types.NamespacedName{
		Namespace: r.operatorNs,
		Name:      LicensingCfgMapName,
	}
	expected := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: nsn.Namespace,
			Name:      nsn.Name,
			Labels: map[string]string{
				common.TypeLabelName: Type,
			},
		},
		Data: info.toMap(),
	}

	reconciled := &corev1.ConfigMap{}
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     r.client,
		Expected:   &expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			return !reflect.DeepEqual(expected.Data, reconciled.Data)
		},
		UpdateReconciled: func() {
			expected.DeepCopyInto(reconciled)
		},
	})
}

// getOperatorLicense gets the operator license.
func (r LicensingResolver) getOperatorLicense() (*license.EnterpriseLicense, error) {
	checker := license.NewLicenseChecker(r.client, r.operatorNs)
	return checker.CurrentEnterpriseLicense()
}

// getOperatorLicenseLevel gets the level of the operator license.
// If no license is given, the defaultOperatorLicenseLevel is returned.
func (r LicensingResolver) getOperatorLicenseLevel(lic *license.EnterpriseLicense) string {
	if lic == nil {
		return defaultOperatorLicenseLevel
	}
	return string(lic.License.Type)
}

// getMaxEnterpriseResourceUnits returns the maximum of enterprise resources units that is allowed for a given license.
// For old style enterprise orchestration licenses which only have max_instances, the maximum of enterprise resources
// units is derived by dividing max_instances by 2.
func (r LicensingResolver) getMaxEnterpriseResourceUnits(lic *license.EnterpriseLicense) int64 {
	if lic == nil {
		return 0
	}

	maxERUs := lic.License.MaxResourceUnits
	if maxERUs == 0 {
		maxERUs = lic.License.MaxInstances / 2
	}

	return int64(maxERUs)
}

// inGB converts a resource.Quantity in gigabytes
func inGB(q resource.Quantity) float64 {
	// divide the value (in bytes) per 1 billion (1GB)
	return float64(q.Value()) / 1000000000
}

// inEnterpriseResourceUnits converts a resource.Quantity to Elastic Enterprise resource units
func inEnterpriseResourceUnits(q resource.Quantity) int64 {
	// divide by the value (in bytes) per 64 billion (64 GB)
	eru := float64(q.Value()) / 64000000000
	// round to the nearest superior integer
	return int64(math.Ceil(eru))
}
