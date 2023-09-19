package virtualmachine

import (
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

func RelocateVirtualMachine(
	vmCtx context.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	relocateSpec types.VirtualMachineRelocateSpec) error {

	vmCtx.Logger.Info("RelocateVirtualMachine", "relocateSpec", relocateSpec)

	task, err := vcVM.Relocate(vmCtx, relocateSpec, types.VirtualMachineMovePriorityDefaultPriority)
	if err != nil {
		return err
	}

	vmCtx.Logger.Info("RelocateVirtualMachine Task", "task", task)

	err = task.Wait(vmCtx)
	if err != nil {
		return err
	}

	vmCtx.Logger.Info("RelocateVirtualMachine Task Wait", "task", task)

	return nil
}
