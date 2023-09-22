// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	goctx "context"
	"fmt"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

// GetResourcePoolByMoID returns the ResourcePool for the MoID.
func GetResourcePoolByMoID(
	ctx goctx.Context,
	finder *find.Finder,
	rpMoID string) (*object.ResourcePool, error) {

	o, err := finder.ObjectReference(ctx, types.ManagedObjectReference{Type: "ResourcePool", Value: rpMoID})
	if err != nil {
		return nil, err
	}

	return o.(*object.ResourcePool), nil
}

func GetResourcePoolByName(
	ctx goctx.Context,
	vimClient *vim25.Client,
	finder *find.Finder,
	rpName string) (*object.ResourcePool, error) {

	o, err := finder.ResourcePool(ctx, rpName)
	if err != nil {
		return nil, err
	}
	return o, nil
}

// GetResourcePoolOwnerMoRef returns the ClusterComputeResource MoID that owns the ResourcePool.
func GetResourcePoolOwnerMoRef(
	ctx goctx.Context,
	vimClient *vim25.Client,
	rpMoID string) (types.ManagedObjectReference, error) {

	rp := object.NewResourcePool(vimClient,
		types.ManagedObjectReference{Type: "ResourcePool", Value: rpMoID})

	objRef, err := rp.Owner(ctx)
	if err != nil {
		return types.ManagedObjectReference{}, err
	}

	return objRef.Reference(), nil
}

// GetChildResourcePool gets the named child ResourcePool from the parent ResourcePool.
func GetChildResourcePool(
	ctx goctx.Context,
	parentRP *object.ResourcePool,
	childName string) (*object.ResourcePool, error) {

	childRP, err := findChildRP(ctx, parentRP, childName)
	if err != nil {
		return nil, err
	} else if childRP == nil {
		return nil, fmt.Errorf("ResourcePool child %q not found under parent ResourcePool %s",
			childName, parentRP.Reference().Value)
	}

	return childRP, nil
}

// DoesChildResourcePoolExist returns if the named child ResourcePool exists under the parent ResourcePool.
func DoesChildResourcePoolExist(
	ctx goctx.Context,
	vimClient *vim25.Client,
	parentRPMoID, childName string) (bool, error) {

	parentRP := object.NewResourcePool(vimClient,
		types.ManagedObjectReference{Type: "ResourcePool", Value: parentRPMoID})

	childRP, err := findChildRP(ctx, parentRP, childName)
	if err != nil {
		return false, err
	}

	return childRP != nil, nil
}

// CreateOrUpdateChildResourcePool creates or updates the child ResourcePool under the parent ResourcePool.
func CreateOrUpdateChildResourcePool(
	ctx goctx.Context,
	vimClient *vim25.Client,
	parentRPMoID string,
	rpSpec *vmopv1.ResourcePoolSpec) (string, error) {

	parentRP := object.NewResourcePool(vimClient,
		types.ManagedObjectReference{Type: "ResourcePool", Value: parentRPMoID})

	childRP, err := findChildRP(ctx, parentRP, rpSpec.Name)
	if err != nil {
		return "", err
	}

	spec := types.DefaultResourceConfigSpec() // TODO Set reservations & limits from rpSpec

	if childRP == nil {
		rp, err := parentRP.Create(ctx, rpSpec.Name, spec)
		if err != nil {
			return "", err
		}

		childRP = rp
	} else { //nolint
		// TODO: 		//       Finish this clause
	}

	return childRP.Reference().Value, nil
}

// DeleteChildResourcePool deletes the child ResourcePool under the parent ResourcePool.
func DeleteChildResourcePool(
	ctx goctx.Context,
	vimClient *vim25.Client,
	parentRPMoID, childName string) error {

	parentRP := object.NewResourcePool(vimClient,
		types.ManagedObjectReference{Type: "ResourcePool", Value: parentRPMoID})

	childRP, err := findChildRP(ctx, parentRP, childName)
	if err != nil || childRP == nil {
		return err
	}

	task, err := childRP.Destroy(ctx)
	if err != nil {
		return err
	}

	if taskResult, err := task.WaitForResult(ctx); err != nil {
		if taskResult == nil || taskResult.Error == nil {
			return err
		}
		return fmt.Errorf("destroy ResourcePool %s task failed: %w: %s",
			childRP.Reference().Value, err, taskResult.Error.LocalizedMessage)
	}

	return nil
}

func findChildRP(
	ctx goctx.Context,
	parentRP *object.ResourcePool,
	childName string) (*object.ResourcePool, error) {

	objRef, err := object.NewSearchIndex(parentRP.Client()).FindChild(ctx, parentRP, childName)
	if err != nil {
		return nil, err
	} else if objRef == nil {
		// FindChild() returns nil when child name is not found.
		return nil, nil
	}

	childRP, ok := objRef.(*object.ResourcePool)
	if !ok {
		return nil, fmt.Errorf("ResourcePool child %q is not a ResourcePool but a %T", childName, objRef)
	}

	return childRP, nil
}
