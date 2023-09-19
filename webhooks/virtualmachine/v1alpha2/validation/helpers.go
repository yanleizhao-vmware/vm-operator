// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

// This file contains v1a2 related changes in other packages that haven't been
// committed yet.

func filterVolumesA2(vm *v1alpha2.VirtualMachine) []v1alpha2.VirtualMachineVolume {
	var volumes []v1alpha2.VirtualMachineVolume
	for _, vol := range vm.Spec.Volumes {
		if pvc := vol.PersistentVolumeClaim; pvc != nil && pvc.InstanceVolumeClaim != nil {
			volumes = append(volumes, vol)
		}
	}

	return volumes
}
