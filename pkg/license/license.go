// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/metrics"
)

const (
	// defaultOperatorLicenseLevel is the default license level when no operator license is installed
	defaultOperatorLicenseLevel = "basic"
	// LicensingCfgMapName is the name of the config map used to store licensing information
	LicensingCfgMapName = "elastic-licensing"
	// Type represents the Elastic usage type used to mark the config map that stores licensing information
	Type = "elastic-usage"
	// GiB represents the number of bytes for 1 GiB
	GiB = 1024 * 1024 * 1024

	elasticsearchKey = "elasticsearch"
	kibanaKey        = "kibana"
	apmKey           = "apm"
	entSearchKey     = "enterprise_search"
	logstashKey      = "logstash"
	totalKey         = "total_managed"
)

type managedMemory struct {
	resource.Quantity
	label string
}

func newManagedMemory(binarySI int64, label string) managedMemory {
	return managedMemory{
		Quantity: *resource.NewQuantity(binarySI, resource.BinarySI),
		label:    label,
	}
}

func (mm managedMemory) inGiB() float64 {
	return inGiB(mm.Quantity)
}

func (mm managedMemory) intoMap(m map[string]string) {
	m[mm.label+"_memory"] = fmt.Sprintf("%0.2fGiB", inGiB(mm.Quantity))
	m[mm.label+"_memory_bytes"] = fmt.Sprintf("%d", mm.Quantity.Value())
}

type memoryUsage struct {
	appUsage    map[string]managedMemory
	totalMemory managedMemory
}

func newMemoryUsage() memoryUsage {
	return memoryUsage{
		appUsage:    map[string]managedMemory{},
		totalMemory: managedMemory{label: totalKey},
	}
}

func (mu *memoryUsage) add(memory managedMemory) {
	mu.appUsage[memory.label] = memory
	mu.totalMemory.Add(memory.Quantity)
}

// LicensingInfo represents information about the operator license including the total memory of all Elastic managed
// components
type LicensingInfo struct {
	memoryUsage
	Timestamp                  string
	EckLicenseLevel            string
	EckLicenseExpiryDate       *time.Time
	MaxEnterpriseResourceUnits int64
	EnterpriseResourceUnits    int64
}

// toMap transforms a LicensingInfo to a map of string, in order to fill in the data of a config map
func (li LicensingInfo) toMap() map[string]string {
	m := map[string]string{
		"timestamp":                 li.Timestamp,
		"eck_license_level":         li.EckLicenseLevel,
		"enterprise_resource_units": strconv.FormatInt(li.EnterpriseResourceUnits, 10),
	}
	for _, v := range li.appUsage {
		v.intoMap(m)
	}
	li.totalMemory.intoMap(m)

	if li.MaxEnterpriseResourceUnits > 0 {
		m["max_enterprise_resource_units"] = strconv.FormatInt(li.MaxEnterpriseResourceUnits, 10)
	}

	if li.EckLicenseExpiryDate != nil {
		m["eck_license_expiry_date"] = li.EckLicenseExpiryDate.Format(time.RFC3339)
	}

	return m
}

func (li LicensingInfo) ReportAsMetrics() {
	labels := prometheus.Labels{metrics.LicenseLevelLabel: li.EckLicenseLevel}
	metrics.LicensingTotalMemoryGauge.With(labels).Set(li.totalMemory.inGiB())
	metrics.LicensingESMemoryGauge.With(labels).Set(li.appUsage[elasticsearchKey].inGiB())
	metrics.LicensingKBMemoryGauge.With(labels).Set(li.appUsage[kibanaKey].inGiB())
	metrics.LicensingAPMMemoryGauge.With(labels).Set(li.appUsage[apmKey].inGiB())
	metrics.LicensingEntSearchMemoryGauge.With(labels).Set(li.appUsage[entSearchKey].inGiB())
	metrics.LicensingLogstashMemoryGauge.With(labels).Set(li.appUsage[logstashKey].inGiB())
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
func (r LicensingResolver) ToInfo(ctx context.Context, memoryUsage memoryUsage) (LicensingInfo, error) {
	operatorLicense, err := r.getOperatorLicense(ctx)
	if err != nil {
		return LicensingInfo{}, err
	}

	licensingInfo := LicensingInfo{
		memoryUsage:             memoryUsage,
		Timestamp:               time.Now().Format(time.RFC3339),
		EckLicenseLevel:         r.getOperatorLicenseLevel(operatorLicense),
		EckLicenseExpiryDate:    r.getOperatorLicenseExpiry(operatorLicense),
		EnterpriseResourceUnits: inEnterpriseResourceUnits(memoryUsage.totalMemory.Quantity),
	}

	// include the max ERUs only for a non trial/basic license
	if maxERUs := r.getMaxEnterpriseResourceUnits(operatorLicense); maxERUs > 0 {
		licensingInfo.MaxEnterpriseResourceUnits = maxERUs
	}

	return licensingInfo, nil
}

// Save updates or creates licensing information in a config map
// This relies on UnconditionalUpdates being supported configmaps and may change in k8s v2: https://github.com/kubernetes/kubernetes/issues/21330
func (r LicensingResolver) Save(ctx context.Context, info LicensingInfo) error {
	span, ctx := apm.StartSpan(ctx, "save_license_info", tracing.SpanTypeApp)
	defer span.End()
	ulog.FromContext(ctx).V(1).Info("Saving", "namespace", r.operatorNs, "configmap_name", LicensingCfgMapName, "license_info", info)
	nsn := types.NamespacedName{
		Namespace: r.operatorNs,
		Name:      LicensingCfgMapName,
	}
	expected := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: nsn.Namespace,
			Name:      nsn.Name,
			Labels: map[string]string{
				commonv1.TypeLabelName: Type,
			},
		},
		Data: info.toMap(),
	}

	reconciled := &corev1.ConfigMap{}
	return reconciler.ReconcileResource(reconciler.Params{
		Context:    ctx,
		Client:     r.client,
		Expected:   &expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			// do not compare timestamp, as it will always change
			expectedData, reconciledData := map[string]string{}, map[string]string{}
			for k, v := range expected.Data {
				expectedData[k] = v
			}
			for k, v := range reconciled.Data {
				reconciledData[k] = v
			}
			delete(expectedData, "timestamp")
			delete(reconciledData, "timestamp")
			return !reflect.DeepEqual(expectedData, reconciledData)
		},
		UpdateReconciled: func() {
			expected.DeepCopyInto(reconciled)
		},
	})
}

// getOperatorLicense gets the operator license.
func (r LicensingResolver) getOperatorLicense(ctx context.Context) (*license.EnterpriseLicense, error) {
	checker := license.NewLicenseChecker(r.client, r.operatorNs)
	return checker.CurrentEnterpriseLicense(ctx)
}

// getOperatorLicenseLevel gets the level of the operator license.
// If no license is given, the defaultOperatorLicenseLevel is returned.
func (r LicensingResolver) getOperatorLicenseLevel(lic *license.EnterpriseLicense) string {
	if lic == nil {
		return defaultOperatorLicenseLevel
	}
	return string(lic.License.Type)
}

// getOperatorLicenseExpiry returns the expiry date of the given Enterprise license or nil.
func (r LicensingResolver) getOperatorLicenseExpiry(lic *license.EnterpriseLicense) *time.Time {
	if lic != nil {
		t := time.Unix(0, lic.License.ExpiryDateInMillis*int64(time.Millisecond))
		return &t
	}
	return nil
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

// inGiB converts a resource.Quantity in gibibytes
func inGiB(q resource.Quantity) float64 {
	// divide the value (in bytes) per 1GiB
	return float64(q.Value()) / (1 * GiB)
}

// inEnterpriseResourceUnits converts a resource.Quantity to Elastic Enterprise resource units
func inEnterpriseResourceUnits(q resource.Quantity) int64 {
	// divide by the value (in bytes) per 64 GiB
	eru := float64(q.Value()) / (64 * GiB)
	// round to the nearest superior integer
	return int64(math.Ceil(eru))
}
