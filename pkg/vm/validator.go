package vm

import (
	"fmt"

	vz "github.com/Code-Hex/vz/v3"
)

// ValidateCPUCount validates the CPU count and returns the validated CPU count.
// If the CPU count is less than the minimum allowed CPU count, the function returns the minimum allowed CPU count and an error.
func ValidateCPUCount(cpuCount uint) (uint, error) {
	maxAllowed := vz.VirtualMachineConfigurationMaximumAllowedCPUCount()
	if cpuCount > maxAllowed {
		return maxAllowed, fmt.Errorf("cpu count %d is greater than the maximum allowed cpu count %d", cpuCount, maxAllowed)
	}

	minAllowed := vz.VirtualMachineConfigurationMinimumAllowedCPUCount()
	if cpuCount < minAllowed {
		return minAllowed, fmt.Errorf("cpu count %d is less than the minimum allowed cpu count %d", cpuCount, minAllowed)
	}

	return cpuCount, nil
}

// ValidateMemorySize validates the memory size and returns the validated memory size.
// If the memory size is less than the minimum allowed memory size, the function returns the minimum allowed memory size and an error.
func ValidateMemorySize(memorySize uint64) (uint64, error) {
	maxAllowed := vz.VirtualMachineConfigurationMaximumAllowedMemorySize()
	if memorySize > maxAllowed {
		return maxAllowed, fmt.Errorf("memory size %d is greater than the maximum allowed memory size %d", memorySize, maxAllowed)
	}

	minAllowed := vz.VirtualMachineConfigurationMinimumAllowedMemorySize()
	if memorySize < minAllowed {
		return minAllowed, fmt.Errorf("memory size %d is less than the minimum allowed memory size %d", memorySize, minAllowed)
	}

	return memorySize, nil
}
