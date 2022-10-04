// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoscaler

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/math"
)

// cpuFromMemory computes a CPU quantity within the specified allowed range by the user proportionally
// to the amount of memory requested by the autoscaling API.
// Invalid memory and CPU ranges with max less than min are rejected during pre-creation/update validation.
func cpuFromMemory(requiredMemoryCapacity resource.Quantity, memoryRange, cpuRange v1alpha1.QuantityRange) resource.Quantity {
	allowedMemoryRange := memoryRange.Max.Value() - memoryRange.Min.Value()
	if allowedMemoryRange == 0 {
		// Can't scale CPU as min and max for memory are equal
		return cpuRange.Min.DeepCopy()
	}
	memRatio := float64(requiredMemoryCapacity.Value()-memoryRange.Min.Value()) / float64(allowedMemoryRange)

	// memory is at its lowest value, return the min value for CPU
	if memRatio == 0 {
		return cpuRange.Min.DeepCopy()
	}
	// memory is at its max value, return the max value for CPU
	if memRatio == 1 {
		return cpuRange.Max.DeepCopy()
	}

	allowedCPURange := float64(cpuRange.Max.MilliValue() - cpuRange.Min.MilliValue())
	requiredAdditionalCPUCapacity := int64(allowedCPURange * memRatio)
	requiredCPUCapacityAsMilli := cpuRange.Min.MilliValue() + requiredAdditionalCPUCapacity

	// Round up CPU to the next core
	requiredCPUCapacityAsMilli = math.RoundUp(requiredCPUCapacityAsMilli, 1000)
	requiredCPUCapacity := resource.NewQuantity(requiredCPUCapacityAsMilli/1000, resource.DecimalSI).DeepCopy()
	if requiredCPUCapacity.Cmp(cpuRange.Max) > 0 {
		requiredCPUCapacity = cpuRange.Max.DeepCopy()
	}
	return requiredCPUCapacity
}

// memoryFromStorage computes a memory quantity within the specified allowed range by the user proportionally
// to the amount of storage requested by the autoscaling API.
// Invalid storage and memory ranges with max less than min are rejected during pre-creation/update validation.
func memoryFromStorage(requiredStorageCapacity resource.Quantity, storageRange, memoryRange v1alpha1.QuantityRange) resource.Quantity {
	allowedStorageRange := storageRange.Max.Value() - storageRange.Min.Value()
	if allowedStorageRange == 0 {
		// Can't scale memory as min and max for storage are equal
		return memoryRange.Min.DeepCopy()
	}
	storageRatio := float64(requiredStorageCapacity.Value()-storageRange.Min.Value()) / float64(allowedStorageRange)
	// storage is at its lowest value, return the min value for memory
	if storageRatio == 0 {
		return memoryRange.Min.DeepCopy()
	}
	// storage is at its maximum value, return the max value for memory
	if storageRatio == 1 {
		return memoryRange.Max.DeepCopy()
	}

	allowedMemoryRange := float64(memoryRange.Max.Value() - memoryRange.Min.Value())
	requiredAdditionalMemoryCapacity := int64(allowedMemoryRange * storageRatio)
	requiredMemoryCapacity := memoryRange.Min.Value() + requiredAdditionalMemoryCapacity

	// Round up memory to the next GiB
	requiredMemoryCapacity = math.RoundUp(requiredMemoryCapacity, v1alpha1.GiB)
	resourceMemoryAsGiga := resource.MustParse(fmt.Sprintf("%dGi", requiredMemoryCapacity/v1alpha1.GiB))

	if resourceMemoryAsGiga.Cmp(memoryRange.Max) > 0 {
		resourceMemoryAsGiga = memoryRange.Max.DeepCopy()
	}
	return resourceMemoryAsGiga
}
