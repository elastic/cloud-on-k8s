// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package resource

import (
	"encoding/json"
	"fmt"
	"time"

	commonlicense "github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/license"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// defaultOperatorLevel is the default license level when no operator license is installed
	defaultOperatorLevel = "basic"
	// minSupportedVersion is the minimum Elastic stack version used to find an operator license
	minSupportedVersion = "6.8.0"
	// licensingCfgMapName is the name of the config map used to store licensing information
	licensingCfgMapName = "elastic-licensing"
)

// LicensingInfo represents information about the operator license including the total memory of all Elastic managed
// components
type LicensingInfo struct {
	Timestamp               string `json:"timestamp"`
	LicenseLevel            string `json:"license_level"`
	MemoryInGiga            string `json:"memory_in_giga"`
	EnterpriseResourceUnits string `json:"enterprise_resource_units"`
}

// LicensingResolver resolves the licensing information of the operator
type LicensingResolver struct {
	operatorNs string
	client     k8s.Client
}

// ToInfo returns licensing information given the total memory of all Elastic managed components
func (r LicensingResolver) ToInfo(totalMemory resource.Quantity) LicensingInfo {
	eru := inEnterpriseResourceUnits(totalMemory)
	memoryInGiga := inGiga(totalMemory)
	licenseLevel := r.getOperatorLicenseLevel()

	return LicensingInfo{
		Timestamp:               time.Now().Format(time.RFC3339),
		LicenseLevel:            licenseLevel,
		MemoryInGiga:            memoryInGiga,
		EnterpriseResourceUnits: eru,
	}
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
// If no license is found, the defaultOperatorLevel is returned.
func (r LicensingResolver) getOperatorLicenseLevel() string {
	checker := commonlicense.NewLicenseChecker(r.client, r.operatorNs)
	minVersion, _ := version.Parse(minSupportedVersion)
	lic, _, found := license.FindLicense(r.client, checker, minVersion)
	if !found {
		return defaultOperatorLevel
	}

	return lic.Type
}

// inGiga converts a resource.Quantity in gigabytes
func inGiga(q resource.Quantity) string {
	// divide the value (in bytes) per 1 billion (1GB)
	return fmt.Sprintf("%0.2f", float32(q.Value())/1000000000)
}

// inEnterpriseResourceUnits converts a resource.Quantity in Elastic Enterprise resource units
func inEnterpriseResourceUnits(q resource.Quantity) string {
	// divide the value in bytes per 64 billion (64 GB)
	return fmt.Sprintf("%0.2f", float32(q.Value())/64000000000)
}

// toMap transforms a LicensingInfo in a map of string, in order to fill in the data of a config map
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
