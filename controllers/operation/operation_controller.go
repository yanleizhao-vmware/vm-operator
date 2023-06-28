package operation

import (
	goctx "context"
	"fmt"
	"io/ioutil"
	"reflect"
	"strings"

	"github.com/go-logr/logr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Reconciler reconciles a Operation object.
type Reconciler struct {
	client.Client
	Logger           logr.Logger
	Recorder         record.Recorder
	VMProvider       vmprovider.VirtualMachineProviderInterface
	maxDeployThreads int
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

func (r *Reconciler) exportVM(ctx goctx.Context, operation *vmopv1.Operation) error {
	logger := log.FromContext(ctx)
	logger.Info("Exporting VM", "Operation", operation)

	// Find the VM referenced by the Operation.
	vm := &vmopv1.VirtualMachine{}
	if err := r.Get(ctx, client.ObjectKey{Name: operation.Spec.EntityName, Namespace: operation.Namespace}, vm); err != nil {
		logger.Error(err, "Failed to find VM referenced by Operation", "Operation", operation)
		return err
	}

	// Add export annotation to VM.
	if vm.Annotations == nil {
		vm.Annotations = make(map[string]string)
	}
	vm.Annotations[vmopv1.ExportAnnotation] = "true"
	if err := r.Update(ctx, vm); err != nil {
		logger.Error(err, "Failed to add export annotation to VM", "VM", vm)
		return err
	}

	// Delete the VM from the cluster.
	if err := r.Delete(ctx, vm); err != nil {
		logger.Error(err, "Failed to delete VM referenced by Operation", "Operation", operation)
		return err
	}

	return nil
}

func (r *Reconciler) importVM(ctx goctx.Context, operation *vmopv1.Operation) error {
	logger := log.FromContext(ctx)
	logger.Info("Importing VM", "Operation", operation)

	kubeconfigPath := "/etc/kubeconfig/config_target_cluster"

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		logger.Error(err, "Failed to build config from flags")
		return err
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		logger.Error(err, "Failed to create dynamic client")
		return err
	}

	gvrOperation := schema.GroupVersionResource{
		Group:    "vmoperator.vmware.com",
		Version:  "v1alpha1",
		Resource: "operations", // use "plans" for the Plan resource
	}

	operationObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "vmoperator.vmware.com/v1alpha1",
			"kind":       "Operation",
			"metadata": map[string]interface{}{
				"name":      "import",
				"namespace": "mobility-service-1",
			},
			"spec": map[string]interface{}{
				"operationType": "Import",
				"entityName":    "centos-cloudinit-example",
				"vmSpec": map[string]interface{}{
					"networkInterfaces": []map[string]interface{}{
						{
							"networkName": "primary-2",
							"networkType": "vsphere-distributed",
						},
					},
					"className":  "best-effort-small",
					"imageName":  "vmi-0992e8a6bf35e6e1f",
					"powerState": "poweredOff",
				},
			},
		},
	}

	_, err = dynamicClient.Resource(gvrOperation).Namespace("mobility-service-1").Create(ctx, operationObj, metav1.CreateOptions{})
	if err != nil {
		logger.Error(err, "Failed to create operation")
		return err
	}

	gvrPlan := schema.GroupVersionResource{
		Group:    "vmoperator.vmware.com",
		Version:  "v1alpha1",
		Resource: "plans", // use "plans" for the Plan resource
	}

	planObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "vmoperator.vmware.com/v1alpha1",
			"kind":       "Plan",
			"metadata": map[string]interface{}{
				"name":      "importvm-plan",
				"namespace": "mobility-service-1",
			},
			"spec": map[string]interface{}{
				"operations": []map[string]interface{}{
					{
						"kind":      "Operation",
						"namespace": "mobility-service-1",
						"name":      "import",
					},
				},
			},
		},
	}

	_, err = dynamicClient.Resource(gvrPlan).Namespace("mobility-service-1").Create(ctx, planObj, metav1.CreateOptions{})
	if err != nil {
		logger.Error(err, "Failed to create plan")
		return err
	}

	return nil
}

func (r *Reconciler) reconcileExport(ctx goctx.Context, operation *vmopv1.Operation) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling export", "Operation", operation)
	if err := r.exportVM(ctx, operation); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) testTargetClusterConnection(ctx goctx.Context) error {
	logger := log.FromContext(ctx)
	kubeconfigPath := "/etc/kubeconfig/config_target_cluster"

	data, err := ioutil.ReadFile(kubeconfigPath)
	if err != nil {
		logger.Error(err, "Failed to read kubeconfig file")
		return err
	}

	logger.Info("Content of the kubeconfig file", "content", string(data))

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		logger.Error(err, "Failed to build config from flags")
		return err
	}

	logger.Info("Config",
		"host", config.Host,
		"APIPath", config.APIPath,
		"Username", config.Username,
		"ServerName", config.TLSClientConfig.ServerName,
		"IsInsecure", config.TLSClientConfig.Insecure,
		"BearerTokenFile", config.BearerTokenFile,
	)

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Error(err, "Failed to create clientset from config")
		return err
	}

	version, err := clientset.Discovery().ServerVersion()
	if err != nil {
		logger.Error(err, "Failed to get server version")
		return err
	}

	logger.Info("Kubernetes Server Version", "version", version.String())
	return nil
}

func (r *Reconciler) reconcileColdMigration(ctx goctx.Context, operation *vmopv1.Operation) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("Reconciling cold migration", "Operation", operation)

	vm := &vmopv1.VirtualMachine{}
	if err := r.Get(ctx, client.ObjectKey{Name: operation.Spec.EntityName, Namespace: operation.Namespace}, vm); err != nil {
		logger.Error(err, "Failed to find VM referenced by Operation", "Operation", operation)
		return ctrl.Result{}, err
	}

	if err := r.testTargetClusterConnection(ctx); err != nil {
		logger.Error(err, "Failed to test target cluster connection")
		return ctrl.Result{}, err
	}

	if err := r.exportVM(ctx, operation); err != nil {
		return ctrl.Result{}, err
	}

	vmCtx := &context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, context.MaxDeployThreadsContextKey, r.maxDeployThreads),
		Logger:  ctrl.Log.WithName("VirtualMachine").WithValues("name", vm.NamespacedName()),
		VM:      vm,
	}

	if err := r.VMProvider.RelocateVirtualMachine(vmCtx, vmCtx.VM); err != nil {
		logger.Error(err, "Failed to relocate VM referenced by Operation", "Operation", operation)
		return ctrl.Result{}, err
	}

	if err := r.importVM(ctx, operation); err != nil {
		return ctrl.Result{}, err
	}

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
		ctx.VMProvider,
		ctx.MaxConcurrentReconciles / (100 / lib.MaxConcurrentCreateVMsOnProvider()),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{MaxConcurrentReconciles: ctx.MaxConcurrentReconciles}).
		Complete(reconciler)
}
