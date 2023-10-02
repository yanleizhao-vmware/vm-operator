package virtualmachine

import (
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

func ReconfigureVirtualMachine(
	vmCtx context.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	reconfigureSpec types.VirtualMachineConfigSpec) error {

	vmCtx.Logger.Info("ReconfigureVirtualMachine", "reconfigureSpec", reconfigureSpec)

	task, err := vcVM.Reconfigure(vmCtx, reconfigureSpec)
	if err != nil {
		return err
	}

	vmCtx.Logger.Info("ReconfigureVirtualMachine Task", "task", task)

	err = task.Wait(vmCtx)
	if err != nil {
		return err
	}

	vmCtx.Logger.Info("ReconfigureVirtualMachine Task Wait", "task", task)

	return nil
}
