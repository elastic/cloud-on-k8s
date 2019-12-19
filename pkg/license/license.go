// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// defaultOperatorLicenseLevel is the default license level when no operator license is installed
	defaultOperatorLicenseLevel = "basic"
	// licensingCfgMapName is the name of the config map used to store licensing information
	licensingCfgMapName = "elastic-licensing"
	// Type represents the Elastic usage type used to mark the config map that stores licensing information
	Type = "elastic-usage"
)

// LicensingInfo represents information about the operator license including the total memory of all Elastic managed
// components
type LicensingInfo struct {
	Timestamp               string `json:"timestamp"`
	EckLicenseLevel         string `json:"eck_license_level"`
	TotalManagedMemory      string `json:"total_managed_memory"`
	EnterpriseResourceUnits string `json:"enterprise_resource_units"`
}

// LicensingResolver resolves the licensing information of the operator
type LicensingResolver struct {
	operatorNs string
	client     k8s.Client
}

// ToInfo returns licensing information given the total memory of all Elastic managed components
func (r LicensingResolver) ToInfo(totalMemory resource.Quantity) (LicensingInfo, error) {
	eru := inEnterpriseResourceUnits(totalMemory)
	memoryInGB := inGB(totalMemory)
	licenseLevel, err := r.getOperatorLicenseLevel()
	if err != nil {
		return LicensingInfo{}, err
	}

	return LicensingInfo{
		Timestamp:               time.Now().Format(time.RFC3339),
		EckLicenseLevel:         licenseLevel,
		TotalManagedMemory:      memoryInGB,
		EnterpriseResourceUnits: eru,
	}, nil
}

// Save updates or creates licensing information in a config map
func (r LicensingResolver) Save(info LicensingInfo, operatorNs string) error {
	data, err := info.toMap()
	if err != nil {
		return err
	}

	log.V(1).Info("Saving", "license_info", info)
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: operatorNs,
			Name:      licensingCfgMapName,
			Labels:    map[string]string{
				common.TypeLabelName: Type,
			},
		},
		Data: data,
	}
	err = r.client.Update(&cm)
	if apierrors.IsNotFound(err) {
		return r.client.Create(&cm)
	}
	return err
}

// getOperatorLicenseLevel gets the level of the operator license.
// If no license is found, the defaultOperatorLicenseLevel is returned.
func (r LicensingResolver) getOperatorLicenseLevel() (string, error) {
	checker := license.NewLicenseChecker(r.client, r.operatorNs)
	lic, err := checker.CurrentEnterpriseLicense()
	if err != nil {
		return "", err
	}

	if lic == nil {
		return defaultOperatorLicenseLevel, nil
	}

	return string(lic.License.Type), nil
}

// inGB converts a resource.Quantity in gigabytes
func inGB(q resource.Quantity) string {
	// divide the value (in bytes) per 1 billion (1GB)
	return fmt.Sprintf("%0.2fGB", float32(q.Value())/1000000000)
}

// inEnterpriseResourceUnits converts a resource.Quantity to Elastic Enterprise resource units
func inEnterpriseResourceUnits(q resource.Quantity) string {
	// divide by the value (in bytes) per 64 billion (64 GB)
	eru := float64(q.Value()) / 64000000000
	// round to the nearest superior integer
	return fmt.Sprintf("%d", int64(math.Ceil(eru)))
}

// toMap transforms a LicensingInfo to a map of string, in order to fill in the data of a config map
func (i LicensingInfo) toMap() (map[string]string, error) {
	bytes, err := json.Marshal(&i)
	if err != nil {
		return nil, err
	}
	var m map[string]string
	err = json.Unmarshal(bytes, &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}
