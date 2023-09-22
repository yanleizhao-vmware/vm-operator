// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	goctx "context"
	"fmt"
	"strings"
	"sync"
	"text/template"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/contentlibrary"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/instancestorage"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/placement"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/storage"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/virtualmachine"
)

// TODO: Until we sort out what the Session becomes.
type vmCreateArgs = session.VMCreateArgs
type vmUpdateArgs = session.VMUpdateArgs
type vmMetadata = session.VMMetadata

const (
	FirstBootDoneAnnotation = "virtualmachine.vmoperator.vmware.com/first-boot-done"
)

var (
	createCountLock       sync.Mutex
	concurrentCreateCount int
)

func (vs *vSphereVMProvider) CreateOrUpdateVirtualMachine(
	ctx goctx.Context,
	vm *vmopv1.VirtualMachine) error {

	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "createOrUpdateVM")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vmCtx.Logger.Info("CreateOrUpdateVirtualMachine", "vm", vm)

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to get VC client")
		return err
	}

	vcVM, err := vs.getVM(vmCtx, client, false)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to get VM")
		return err
	}

	vmCtx.Logger.Info("vcVM", "vcVM", vcVM)

	if vcVM == nil {
		vcVM, err = vs.createVirtualMachine(vmCtx, client)
		if err != nil {
			vmCtx.Logger.Error(err, "Failed to create VM")
			return err
		}

		if vcVM == nil {
			// Creation was not ready or blocked for some reason. We depend on the controller
			// to eventually retry the create.
			vmCtx.Logger.Info("VM creation not ready")
			return nil
		}
	}

	return vs.updateVirtualMachine(vmCtx, vcVM, client)
}

func GetNetworkFromVM(vmCtx *context.VirtualMachineContext, vm *object.VirtualMachine) (*types.VirtualVmxnet3, error) {
	vdl, err := vm.Device(vmCtx)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to find devices of virtual machine")
	}

	for _, d := range vdl {
		if nic, ok := d.(*types.VirtualVmxnet3); ok {
			// Be happy with the first NIC for now.
			return nic, nil
		}
	}

	return nil, errors.New("VM doesn't have a network card")
}

func (vs *vSphereVMProvider) RelocateVirtualMachine(ctx goctx.Context, vm *vmopv1.VirtualMachine, supervisorRelocateSpec *vmopv1.RelocateSpec) error {
	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "relocateVM")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		return err
	}

	vcVM, err := vs.getVM(vmCtx, client, false)
	if err != nil {
		return err
	} else if vcVM == nil {
		// VM does not exist.
		return nil
	}
	vmCtx.Logger.Info("Relocating VM", "vm", vcVM.Reference())

	dstHost, err := client.Finder().HostSystem(vmCtx, supervisorRelocateSpec.HostIp)
	if err != nil {
		return err
	}
	dstHostMoRef := dstHost.Reference()
	vmCtx.Logger.Info("Dst host", "host", dstHostMoRef)

	dstPool, err := client.Finder().ResourcePool(vmCtx, supervisorRelocateSpec.ResourcePoolName)
	if err != nil {
		return err
	}
	dstPoolMoRef := dstPool.Reference()
	vmCtx.Logger.Info("Dst resource pool", "rp", dstPoolMoRef)

	dstDs, err := client.Finder().Datastore(vmCtx, supervisorRelocateSpec.DatastoreName)
	if err != nil {
		return err
	}
	dstDsMoRef := dstDs.Reference()
	vmCtx.Logger.Info("Dst datastore", "ds", dstDsMoRef)

	dstVMNetworkName := "primary-vds-2"
	dstNetwork, err := client.Finder().Network(vmCtx, supervisorRelocateSpec.VmNetworkName)
	if err != nil {
		return err
	}
	dstNetworkMoRef := dstNetwork.Reference()
	vmCtx.Logger.Info("Dst network", "network", dstNetworkMoRef)
	srcVMNetwork, err := GetNetworkFromVM(&vmCtx, vcVM)
	if err != nil {
		return err
	}
	var deviceConfigSpecs []types.BaseVirtualDeviceConfigSpec
	deviceConfigSpec := &types.VirtualDeviceConfigSpec{
		Operation: types.VirtualDeviceConfigSpecOperationEdit,
		Device:    srcVMNetwork,
	}
	deviceConfigSpec.Device.GetVirtualDevice().Backing = &types.VirtualEthernetCardNetworkBackingInfo{
		VirtualDeviceDeviceBackingInfo: types.VirtualDeviceDeviceBackingInfo{
			DeviceName: dstVMNetworkName,
		},
	}
	deviceConfigSpecs = append(deviceConfigSpecs, deviceConfigSpec)

	dstFolder, err := client.Finder().Folder(vmCtx, supervisorRelocateSpec.FolderName)
	if err != nil {
		return err
	}
	dstFolderMoRef := dstFolder.Reference()
	vmCtx.Logger.Info("Dst folder", "folder", dstFolderMoRef)

	relocateSpec := types.VirtualMachineRelocateSpec{
		Folder:       &dstFolderMoRef,
		Datastore:    &dstDsMoRef,
		Pool:         &dstPoolMoRef,
		Host:         &dstHostMoRef,
		DeviceChange: deviceConfigSpecs,
	}

	return virtualmachine.RelocateVirtualMachine(vmCtx, vcVM, relocateSpec)
}

func (vs *vSphereVMProvider) DeleteVirtualMachine(
	ctx goctx.Context,
	vm *vmopv1.VirtualMachine) error {

	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "deleteVM")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		return err
	}

	vcVM, err := vs.getVM(vmCtx, client, false)
	if err != nil {
		return err
	} else if vcVM == nil {
		// VM does not exist.
		return nil
	}

	return virtualmachine.DeleteVirtualMachine(vmCtx, vcVM)
}

func (vs *vSphereVMProvider) PublishVirtualMachine(ctx goctx.Context, vm *vmopv1.VirtualMachine,
	vmPub *vmopv1.VirtualMachinePublishRequest, cl *imgregv1a1.ContentLibrary, actID string) (string, error) {
	vmCtx := context.VirtualMachineContext{
		Context: ctx,
		// Update logger info
		Logger: log.WithValues("vmName", vm.NamespacedName()).
			WithValues("clName", fmt.Sprintf("%s/%s", cl.Namespace, cl.Name)).
			WithValues("vmPubName", fmt.Sprintf("%s/%s", vmPub.Namespace, vmPub.Name)),
		VM: vm,
	}

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get vCenter client")
	}

	itemID, err := virtualmachine.CreateOVF(vmCtx, client.RestClient(), vmPub, cl, actID)
	if err != nil {
		return "", err
	}
	return itemID, nil
}

// BackupVirtualMachine backs up the VM data required for restore.
func (vs *vSphereVMProvider) BackupVirtualMachine(ctx goctx.Context, vm *vmopv1.VirtualMachine) error {
	client, err := vs.getVcClient(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get vCenter client for backing up Virtual Machine")
	}

	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "backupVM")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vcVM, err := vs.getVM(vmCtx, client, true)
	if err != nil {
		return errors.Wrapf(err, "failed to get vSphere Virtual Machine for backing up")
	}

	vmMetadata, err := GetVMMetadata(vmCtx, vs.k8sClient)
	if err != nil {
		return errors.Wrapf(err, "failed to get VM metadata for backing up")
	}

	return virtualmachine.BackupVirtualMachine(vmCtx, vcVM, vmMetadata.Data)
}

func (vs *vSphereVMProvider) GetVirtualMachineGuestHeartbeat(
	ctx goctx.Context,
	vm *vmopv1.VirtualMachine) (vmopv1.GuestHeartbeatStatus, error) {

	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "heartbeat")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		return "", err
	}

	vcVM, err := vs.getVM(vmCtx, client, true)
	if err != nil {
		return "", err
	}

	status, err := virtualmachine.GetGuestHeartBeatStatus(vmCtx, vcVM)
	if err != nil {
		return "", err
	}

	return status, nil
}

func (vs *vSphereVMProvider) GetVirtualMachineWebMKSTicket(
	ctx goctx.Context,
	vm *vmopv1.VirtualMachine,
	pubKey string) (string, error) {

	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "webconsole")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		return "", err
	}

	vcVM, err := vs.getVM(vmCtx, client, true)
	if err != nil {
		return "", err
	}

	ticket, err := virtualmachine.GetWebConsoleTicket(vmCtx, vcVM, pubKey)
	if err != nil {
		return "", err
	}

	return ticket, nil
}

func (vs *vSphereVMProvider) GetVirtualMachineHardwareVersion(
	ctx goctx.Context,
	vm *vmopv1.VirtualMachine) (int32, error) {

	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "hardware-version")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		return 0, err
	}

	vcVM, err := vs.getVM(vmCtx, client, true)
	if err != nil {
		return 0, err
	}

	var o mo.VirtualMachine
	err = vcVM.Properties(vmCtx, vcVM.Reference(), []string{"config.version"}, &o)
	if err != nil {
		return 0, err
	}

	return contentlibrary.ParseVirtualHardwareVersion(o.Config.Version), nil
}

func (vs *vSphereVMProvider) createVirtualMachine(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client) (*object.VirtualMachine, error) {

	createArgs, err := vs.vmCreateGetArgs(vmCtx, vcClient)
	if err != nil {
		return nil, err
	}

	// Historically this is about the point when we say we're creating but there
	// are still several steps before then.
	vmCtx.VM.Status.Phase = vmopv1.Creating

	err = vs.vmCreateDoPlacement(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, err
	}

	err = vs.vmCreateGetFolderAndRPMoIDs(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, err
	}

	err = vs.vmCreateIsReady(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, err
	}

	// BMV: This is about where we used to do this check but it prb make more sense
	// to do earlier, as to limit wasted work.
	maxDeployThreads, ok := vmCtx.Value(context.MaxDeployThreadsContextKey).(int)
	if !ok {
		return nil, fmt.Errorf("MaxDeployThreadsContextKey missing from context")
	}

	allowed, createDeferFn := vs.vmCreateConcurrentAllowed(vmCtx, maxDeployThreads)
	if !allowed {
		return nil, nil
	}
	defer createDeferFn()

	var vcVM *object.VirtualMachine
	{
		// Hack - create just enough of the Session that's needed for create
		vmCtx.Logger.Info("Creating VirtualMachine")

		ses := &session.Session{
			K8sClient: vs.k8sClient,
			Client:    vcClient,
			Finder:    vcClient.Finder(),
		}

		vcVM, err = ses.CreateVirtualMachine(vmCtx, createArgs)
		if err != nil {
			vmCtx.Logger.Error(err, "CreateVirtualMachine failed")
			return nil, err
		}

		// Set a few Status fields that we easily have on hand here. We will immediately call
		// updateVirtualMachine() next which will set it all.
		vmCtx.VM.Status.Phase = vmopv1.Created
		vmCtx.VM.Status.UniqueID = vcVM.Reference().Value
	}

	return vcVM, nil
}

func (vs *vSphereVMProvider) updateVirtualMachine(
	vmCtx context.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	vcClient *vcclient.Client) error {

	vmCtx.Logger.V(4).Info("Updating VirtualMachine")
	{
		// Hack - create just enough of the Session that's needed for update

		cluster, err := virtualmachine.GetVMClusterComputeResource(vmCtx, vcVM)
		if err != nil {
			vmCtx.Logger.Error(err, "GetVMClusterComputeResource failed")
			return err
		}

		ses := &session.Session{
			K8sClient: vs.k8sClient,
			Client:    vcClient,
			Finder:    vcClient.Finder(),
			Cluster:   cluster,
		}
		ses.NetworkProvider = network.NewProvider(ses.K8sClient, ses.Client.VimClient(), ses.Finder, ses.Cluster)

		getUpdateArgsFn := func() (*vmUpdateArgs, error) {
			return vs.vmUpdateGetArgs(vmCtx)
		}

		err = ses.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgsFn)
		if err != nil {
			vmCtx.Logger.Error(err, "UpdateVirtualMachine failed")
			return err
		}
	}

	return nil
}

// vmCreateDoPlacement determines placement of the VM prior to creating the VM on VC.
func (vs *vSphereVMProvider) vmCreateDoPlacement(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *vmCreateArgs) error {

	result, err := placement.Placement(vmCtx, vs.k8sClient, vcClient.VimClient(),
		createArgs.PlacementConfigSpec, createArgs.ChildResourcePoolName)
	if err != nil {
		return err
	}

	if result.PoolMoRef.Value != "" {
		createArgs.ResourcePoolMoID = result.PoolMoRef.Value
	}

	if result.HostMoRef != nil {
		createArgs.HostMoID = result.HostMoRef.Value
	}

	if result.InstanceStoragePlacement {
		hostMoID := createArgs.HostMoID

		if hostMoID == "" {
			return fmt.Errorf("placement result missing host required for instance storage")
		}

		hostFQDN, err := vcenter.GetESXHostFQDN(vmCtx, vcClient.VimClient(), hostMoID)
		if err != nil {
			return err
		}

		if vmCtx.VM.Annotations == nil {
			vmCtx.VM.Annotations = map[string]string{}
		}
		vmCtx.VM.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey] = hostMoID
		vmCtx.VM.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey] = hostFQDN
	}

	if result.ZonePlacement {
		if vmCtx.VM.Labels == nil {
			vmCtx.VM.Labels = map[string]string{}
		}
		vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey] = result.ZoneName
	}

	return nil
}

// vmCreateGetFolderAndRPMoIDs gets the MoIDs of the Folder and Resource Pool the VM will be created under.
func (vs *vSphereVMProvider) vmCreateGetFolderAndRPMoIDs(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *vmCreateArgs) error {

	if createArgs.ResourcePoolMoID == "" {
		// We did not do placement so find this namespace/zone ResourcePool and Folder.

		nsFolderMoID, rpMoID, err := topology.GetNamespaceFolderAndRPMoID(vmCtx, vs.k8sClient,
			vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey], vmCtx.VM.Namespace)
		if err != nil {
			return err
		}

		// If this VM has a ResourcePolicy ResourcePool, lookup the child ResourcePool under the
		// namespace/zone's root ResourcePool. This will be the VM's ResourcePool.
		if createArgs.ChildResourcePoolName != "" {
			parentRP := object.NewResourcePool(vcClient.VimClient(),
				types.ManagedObjectReference{Type: "ResourcePool", Value: rpMoID})

			childRP, err := vcenter.GetChildResourcePool(vmCtx, parentRP, createArgs.ChildResourcePoolName)
			if err != nil {
				return err
			}

			rpMoID = childRP.Reference().Value
		}

		createArgs.ResourcePoolMoID = rpMoID
		createArgs.FolderMoID = nsFolderMoID

	} else {
		// Placement already selected the ResourcePool/Cluster, so we just need this namespace's folder.
		nsFolderMoID, err := topology.GetNamespaceFolderMoID(vmCtx, vs.k8sClient, vmCtx.VM.Namespace)
		if err != nil {
			return err
		}

		createArgs.FolderMoID = nsFolderMoID
	}

	// If this VM has a ResourcePolicy Folder, lookup the child Folder under the namespace's Folder.
	// This will be the VM's parent Folder in the VC inventory.
	if createArgs.ChildFolderName != "" {
		parentFolder := object.NewFolder(vcClient.VimClient(),
			types.ManagedObjectReference{Type: "Folder", Value: createArgs.FolderMoID})

		childFolder, err := vcenter.GetChildFolder(vmCtx, parentFolder, createArgs.ChildFolderName)
		if err != nil {
			return err
		}

		createArgs.FolderMoID = childFolder.Reference().Value
	}

	return nil
}

func (vs *vSphereVMProvider) vmCreateIsReady(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *vmCreateArgs) error {

	if policy := createArgs.ResourcePolicy; policy != nil {
		if !policy.DeletionTimestamp.IsZero() {
			return fmt.Errorf("cannot create VirtualMachine when its resource policy is being deleted")
		}

		clusterMoRef, err := vcenter.GetResourcePoolOwnerMoRef(vmCtx, vcClient.VimClient(), createArgs.ResourcePoolMoID)
		if err != nil {
			return err
		}

		// TODO: May want to do this as to filter the placement candidates.
		exists, err := vs.doClusterModulesExist(vmCtx, vcClient.ClusterModuleClient(), clusterMoRef, policy)
		if err != nil {
			return err
		} else if !exists {
			return fmt.Errorf("VirtualMachineSetResourcePolicy cluster modules is not ready")
		}
	}

	if createArgs.HasInstanceStorage {
		if _, ok := vmCtx.VM.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey]; !ok {
			return fmt.Errorf("instance storage PVCs are not bound yet")
		}
	}

	return nil
}

func (vs *vSphereVMProvider) vmCreateConcurrentAllowed(
	vmCtx context.VirtualMachineContext,
	maxDeployThreads int) (bool, func()) {

	createCountLock.Lock()
	if concurrentCreateCount >= maxDeployThreads {
		createCountLock.Unlock()
		vmCtx.Logger.Info("Too many create VirtualMachine already occurring. Re-queueing request")
		return false, nil
	}

	concurrentCreateCount++
	createCountLock.Unlock()

	decrementFn := func() {
		createCountLock.Lock()
		concurrentCreateCount--
		createCountLock.Unlock()
	}

	return true, decrementFn
}

func (vs *vSphereVMProvider) vmCreateGetArgs(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client) (*vmCreateArgs, error) {

	createArgs, err := vs.vmCreateGetPrereqs(vmCtx)
	if err != nil {
		return nil, err
	}

	if lib.IsInstanceStorageFSSEnabled() {
		// This must be done here so the instance storage volumes are present so the next
		// step can fetch all the storage profiles.
		if err := AddInstanceStorageVolumes(vmCtx, createArgs.VMClass); err != nil {
			return nil, err
		}

		createArgs.HasInstanceStorage = instancestorage.IsConfigured(vmCtx.VM)
	}

	err = vs.vmCreateGetStoragePrereqs(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, err
	}

	vs.vmCreateGenConfigSpec(vmCtx, createArgs)

	err = vs.vmCreateValidateArgs(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, err
	}

	return createArgs, nil
}

// vmCreateGetPrereqs returns the vmCreateArgs populated with the k8s objects required to
// create the VM on VC.
func (vs *vSphereVMProvider) vmCreateGetPrereqs(
	vmCtx context.VirtualMachineContext) (*vmCreateArgs, error) {

	vmClass, err := GetVirtualMachineClass(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	vmImageSpec, vmImageStatus, clUUID, err := GetVMImageAndContentLibraryUUID(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	resourcePolicy, err := GetVMSetResourcePolicy(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	vmMD, err := GetVMMetadata(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	createArgs := &vmCreateArgs{}
	createArgs.VMClass = vmClass
	createArgs.VMImageSpec = vmImageSpec
	createArgs.VMImageStatus = vmImageStatus
	createArgs.ContentLibraryUUID = clUUID
	createArgs.VMMetadata = vmMD

	// TODO: Perhaps a condition type for each resource is better so all missing one(s)
	// 	     can be reported at once (and will help for the best-effort update changes).
	// This is about where historically we set this condition but there are still a lot
	// more checks to go.
	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachinePrereqReadyCondition)

	if resourcePolicy != nil {
		rp := resourcePolicy.Spec.ResourcePool

		createArgs.ResourcePolicy = resourcePolicy
		createArgs.ChildFolderName = resourcePolicy.Spec.Folder.Name
		createArgs.ChildResourcePoolName = rp.Name
	}

	if res := vmClass.Spec.Policies.Resources; !res.Requests.Cpu.IsZero() || !res.Limits.Cpu.IsZero() {
		freq, err := vs.getOrComputeCPUMinFrequency(vmCtx)
		if err != nil {
			return nil, err
		}
		createArgs.MinCPUFreq = freq
	}

	if lib.IsVMClassAsConfigFSSDaynDateEnabled() {
		if cs := createArgs.VMClass.Spec.ConfigSpec; cs != nil {
			classConfigSpec, err := GetVMClassConfigSpec(cs)
			if err != nil {
				return nil, err
			}
			createArgs.ClassConfigSpec = classConfigSpec
		}
	}

	return createArgs, nil
}

func (vs *vSphereVMProvider) vmCreateGetStoragePrereqs(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *vmCreateArgs) error {

	storageClassesToIDs, err := storage.GetVMStoragePoliciesIDs(vmCtx, vs.k8sClient)
	if err != nil {
		return err
	}

	vmStorageProfileID := storageClassesToIDs[vmCtx.VM.Spec.StorageClass]

	provisioningType, err := virtualmachine.GetDefaultDiskProvisioningType(vmCtx, vcClient, vmStorageProfileID)
	if err != nil {
		return err
	}

	createArgs.StorageClassesToIDs = storageClassesToIDs
	createArgs.StorageProvisioning = provisioningType
	createArgs.StorageProfileID = vmStorageProfileID

	return nil
}

func (vs *vSphereVMProvider) vmCreateGenConfigSpec(
	vmCtx context.VirtualMachineContext,
	createArgs *vmCreateArgs) {

	var vmClassConfigSpec *types.VirtualMachineConfigSpec
	if createArgs.ClassConfigSpec != nil {
		// With DaynDate FFS, the VM created is based on the VMClass ConfigSpec. Otherwise, the VMClass
		// ConfigSpec is handled during the post-create Update.
		if lib.IsVMClassAsConfigFSSDaynDateEnabled() {
			t := *createArgs.ClassConfigSpec
			// Remove the NICs since we don't know the backing yet; they'll be added in the post-create Update.
			util.RemoveDevicesFromConfigSpec(&t, util.IsEthernetCard)
			vmClassConfigSpec = &t
		}
	}

	// Create both until we can sanitize the placement ConfigSpec for creation.

	createArgs.ConfigSpec = virtualmachine.CreateConfigSpec(
		vmCtx.VM.Name,
		&createArgs.VMClass.Spec,
		createArgs.MinCPUFreq,
		createArgs.VMImageStatus.Firmware,
		vmClassConfigSpec)

	createArgs.PlacementConfigSpec = virtualmachine.CreateConfigSpecForPlacement(
		vmCtx,
		&createArgs.VMClass.Spec,
		createArgs.MinCPUFreq,
		createArgs.StorageClassesToIDs,
		createArgs.VMImageStatus.Firmware,
		vmClassConfigSpec)

	// Set a hardware version in the create config spec when VMs are created with PVCs/PCI(vGPU and DDPIO) devices
	// and VMClass config spec has an empty hardware version.
	if lib.IsVMClassAsConfigFSSDaynDateEnabled() && createArgs.ConfigSpec.Version == "" {
		if version := HardwareVersionForPVCandPCIDevices(createArgs.VMImageSpec.HardwareVersion,
			createArgs.ConfigSpec, HasPVC(vmCtx.VM.Spec)); version != 0 {
			createArgs.ConfigSpec.Version = fmt.Sprintf("vmx-%d", version)
		}
	}
}

func (vs *vSphereVMProvider) vmCreateValidateArgs(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *vmCreateArgs) error {

	// Some of this would be better done in the validation webhook but have it here for now.
	cfg := vcClient.Config()

	if cfg.StorageClassRequired {
		// In WCP this is always required.
		if vmCtx.VM.Spec.StorageClass == "" {
			return fmt.Errorf("StorageClass is required but not specified")
		}

		if createArgs.StorageProfileID == "" {
			// GetVMStoragePoliciesIDs() would have returned an error if the policy didn't exist, but
			// ensure the field is set.
			return fmt.Errorf("no StorageProfile found for StorageClass %s", vmCtx.VM.Spec.StorageClass)
		}

	} else if vmCtx.VM.Spec.StorageClass == "" {
		// This is only set in gce2e.
		if cfg.Datastore == "" {
			return fmt.Errorf("no Datastore provided in configuration")
		}

		datastore, err := vcClient.Finder().Datastore(vmCtx, cfg.Datastore)
		if err != nil {
			return fmt.Errorf("failed to find Datastore %s: %w", cfg.Datastore, err)
		}

		createArgs.DatastoreMoID = datastore.Reference().Value
	}

	return nil
}

func (vs *vSphereVMProvider) vmUpdateGetArgs(
	vmCtx context.VirtualMachineContext) (*vmUpdateArgs, error) {

	vmClass, err := GetVirtualMachineClass(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	resourcePolicy, err := GetVMSetResourcePolicy(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	vmMD, err := GetVMMetadata(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	updateArgs := &vmUpdateArgs{}
	updateArgs.VMClass = vmClass
	updateArgs.ResourcePolicy = resourcePolicy
	updateArgs.VMMetadata = vmMD

	// We're always ready - again - at this point since we've fetched the above objects. We really should
	// not be touching this condition after creation but that is for another day.
	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachinePrereqReadyCondition)

	if res := vmClass.Spec.Policies.Resources; !res.Requests.Cpu.IsZero() || !res.Limits.Cpu.IsZero() {
		freq, err := vs.getOrComputeCPUMinFrequency(vmCtx)
		if err != nil {
			return nil, err
		}
		updateArgs.MinCPUFreq = freq
	}

	if lib.IsVMClassAsConfigFSSDaynDateEnabled() {
		if cs := updateArgs.VMClass.Spec.ConfigSpec; cs != nil {
			classConfigSpec, err := GetVMClassConfigSpec(cs)
			if err != nil {
				return nil, err
			}
			updateArgs.ClassConfigSpec = classConfigSpec
		}
	}

	imageFirmware := ""
	// Only get VM image when this is the VM first boot.
	if isVMFirstBoot(vmCtx) {
		_, vmImageStatus, _, err := GetVMImageAndContentLibraryUUID(vmCtx, vs.k8sClient)
		if err != nil {
			return nil, err
		}
		imageFirmware = vmImageStatus.Firmware

		// The only use of this is for the global JSON_EXTRA_CONFIG to set the image name.
		// The global extra config should only be set during first boot.
		renderTemplateFn := func(name, text string) string {
			t, err := template.New(name).Parse(text)
			if err != nil {
				return text
			}
			b := strings.Builder{}
			if err := t.Execute(&b, vmImageStatus); err != nil {
				return text
			}
			return b.String()
		}
		extraConfig := make(map[string]string, len(vs.globalExtraConfig))
		for k, v := range vs.globalExtraConfig {
			extraConfig[k] = renderTemplateFn(k, v)
		}

		// Enabling the defer-cloud-init extraConfig key for V1Alpha1Compatible images defers cloud-init from running on first boot
		// and disables networking configurations by cloud-init. Therefore, only set the extraConfig key to enabled
		// when the vmMetadata is nil or when the transport requested is not CloudInit.
		if conditions.IsTrueFromConditions(vmImageStatus.Conditions,
			vmopv1.VirtualMachineImageV1Alpha1CompatibleCondition) {
			updateArgs.VirtualMachineImageV1Alpha1Compatible = true
		}
		updateArgs.ExtraConfig = extraConfig
	}

	updateArgs.ConfigSpec = virtualmachine.CreateConfigSpec(
		vmCtx.VM.Name,
		&updateArgs.VMClass.Spec,
		updateArgs.MinCPUFreq,
		imageFirmware,
		updateArgs.ClassConfigSpec)

	return updateArgs, nil
}

func isVMFirstBoot(vmCtx context.VirtualMachineContext) bool {
	if _, ok := vmCtx.VM.Annotations[FirstBootDoneAnnotation]; ok {
		return false
	}

	return true
}
