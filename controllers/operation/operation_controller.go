package operation

import (
	goctx "context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Reconciler reconciles a Operation object.
type Reconciler struct {
	client.Client
	Logger   logr.Logger
	Recorder record.Recorder
}

func (r *Reconciler) reconcileImport(ctx goctx.Context, operation *vmopv1.Operation) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling import", "Operation", operation)

	// Find the VM referenced by the Operation
	vm := &vmopv1.VirtualMachine{}
	if err := r.Get(ctx, client.ObjectKey{Name: operation.Spec.EntityName, Namespace: operation.Namespace}, vm); err != nil {
		// Create a new VM.
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operation.Spec.EntityName,
				Namespace: operation.Namespace,
			},
			Spec: operation.Spec.VmSpec,
		}
		if err := r.Create(ctx, vm); err != nil {
			logger.Error(err, "Failed to create VM referenced by Operation", "Operation", operation)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	// Exit since VM already exists.
	logger.Info("VM already exists", "VM", vm)
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileExport(ctx goctx.Context, operation *vmopv1.Operation) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling export", "Operation", operation)

	// Find the VM referenced by the Operation
	vm := &vmopv1.VirtualMachine{}
	if err := r.Get(ctx, client.ObjectKey{Name: operation.Spec.EntityName, Namespace: operation.Namespace}, vm); err != nil {
		logger.Error(err, "Failed to find VM referenced by Operation", "Operation", operation)
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling export", "VM", vm)

	// Add export annotation to VM
	if vm.Annotations == nil {
		vm.Annotations = make(map[string]string)
	}
	vm.Annotations[vmopv1.ExportAnnotation] = "true"
	if err := r.Update(ctx, vm); err != nil {
		logger.Error(err, "Failed to add export annotation to VM", "VM", vm)
		return ctrl.Result{}, err
	}

	// Delete the VM
	if err := r.Delete(ctx, vm); err != nil {
		logger.Error(err, "Failed to delete VM referenced by Operation", "Operation", operation)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileColdMigration(ctx goctx.Context, operation *vmopv1.Operation) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileLiveMigration(ctx goctx.Context, operation *vmopv1.Operation) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Operation", "request", req)

	Operation := &vmopv1.Operation{}
	if err := r.Get(ctx, req.NamespacedName, Operation); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling Operation", "Operation", Operation)

	switch Operation.Spec.OperationType {
	case vmopv1.Import:
		return r.reconcileImport(ctx, Operation)
	case vmopv1.Export:
		return r.reconcileExport(ctx, Operation)
	case vmopv1.ColdMigration:
		return r.reconcileColdMigration(ctx, Operation)
	case vmopv1.LiveMigration:
		return r.reconcileLiveMigration(ctx, Operation)
	default:
		return ctrl.Result{}, nil
	}
}

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr ctrl.Manager) error {

	var (
		controlledType     = &vmopv1.Operation{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	reconciler := &Reconciler{
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{MaxConcurrentReconciles: ctx.MaxConcurrentReconciles}).
		Complete(reconciler)
}
