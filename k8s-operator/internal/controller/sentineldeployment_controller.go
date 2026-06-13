package controller

import (
	"context"
	"fmt"
	"math"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	velov1alpha1 "github.com/velo-infra-playground/k8s-operator/api/v1alpha1"
)

// SentinelDeploymentReconciler reconciles a SentinelDeployment object
type SentinelDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=velo.infra,resources=sentineldeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=velo.infra,resources=sentineldeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=velo.infra,resources=sentineldeployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *SentinelDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Fetch the SentinelDeployment custom resource
	var sentinelDep velov1alpha1.SentinelDeployment
	if err := r.Get(ctx, req.NamespacedName, &sentinelDep); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("SentinelDeployment resource not found. Skipping reconcile.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get SentinelDeployment resource.")
		return ctrl.Result{}, err
	}

	// 2. Resolve parameters with defaults
	minReplicas := sentinelDep.Spec.MinReplicas
	if minReplicas <= 0 {
		minReplicas = 1
	}
	maxReplicas := sentinelDep.Spec.MaxReplicas
	if maxReplicas <= 0 {
		maxReplicas = 10
	}
	scaleThreshold := sentinelDep.Spec.ScaleThreshold
	if scaleThreshold <= 0 {
		scaleThreshold = 5
	}

	// 3. Query Active gRPC Queue Depth from Gateway
	queueDepth, err := r.getQueueDepth(sentinelDep.Spec.GatewayAddress)
	if err != nil {
		logger.Error(err, "Failed to retrieve queue depth from sentinel gateway. Using fallback.")
		queueDepth = 0
	}

	// 4. Calculate target replicas based on queue depth
	// Target = Ceil(QueueDepth / Threshold)
	targetReplicas := int32(math.Ceil(float64(queueDepth) / float64(scaleThreshold)))
	if targetReplicas < minReplicas {
		targetReplicas = minReplicas
	}
	if targetReplicas > maxReplicas {
		targetReplicas = maxReplicas
	}

	logger.Info(fmt.Sprintf("Queue check: Depth=%d, Threshold=%d. Target replicas calculation: %d", 
		queueDepth, scaleThreshold, targetReplicas))

	// 5. Fetch the target deployment to scale
	var targetDep appsv1.Deployment
	depKey := client.ObjectKey{
		Namespace: req.Namespace,
		Name:      sentinelDep.Spec.TargetDeployment,
	}

	depFound := true
	if err := r.Get(ctx, depKey, &targetDep); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("Target Deployment %s not found in namespace %s. Delaying scale.", 
				sentinelDep.Spec.TargetDeployment, req.Namespace))
			depFound = false
		} else {
			logger.Error(err, "Failed to get target Deployment.")
			return ctrl.Result{}, err
		}
	}

	scaled := false
	if depFound {
		currentReplicas := int32(1)
		if targetDep.Spec.Replicas != nil {
			currentReplicas = *targetDep.Spec.Replicas
		}

		if currentReplicas != targetReplicas {
			// Update the deployment replicas
			targetDep.Spec.Replicas = &targetReplicas
			if err := r.Update(ctx, &targetDep); err != nil {
				logger.Error(err, "Failed to update target Deployment replica count.")
				return ctrl.Result{}, err
			}
			logger.Info(fmt.Sprintf("Successfully scaled Deployment %s from %d to %d replicas", 
				targetDep.Name, currentReplicas, targetReplicas))
			scaled = true
		}
	}

	// 6. Update Custom Resource Status
	currentStatusReplicas := int32(0)
	if depFound {
		currentStatusReplicas = targetDep.Status.Replicas
	}

	statusChanged := sentinelDep.Status.Replicas != currentStatusReplicas || 
		sentinelDep.Status.ActiveQueueDepth != queueDepth || 
		scaled

	if statusChanged {
		sentinelDep.Status.Replicas = currentStatusReplicas
		sentinelDep.Status.ActiveQueueDepth = queueDepth
		if scaled {
			now := metav1.NewTime(time.Now())
			sentinelDep.Status.LastScaleTime = &now
		}

		if err := r.Status().Update(ctx, &sentinelDep); err != nil {
			logger.Error(err, "Failed to update SentinelDeployment status.")
			return ctrl.Result{}, err
		}
	}

	// Requeue to check periodically (default every 10 seconds for simulator reactive demo)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// getQueueDepth queries the gateway queue depth.
// If address is "mock" or cannot be reached, it simulates a time-based queue fluctuation.
func (r *SentinelDeploymentReconciler) getQueueDepth(gatewayAddr string) (int32, error) {
	if gatewayAddr == "" || gatewayAddr == "mock" || gatewayAddr == "localhost:9000" {
		// Mock logic: fluctuate queue depth between 2, 8, and 18 every 30 seconds
		nowSec := time.Now().Unix()
		cycle := (nowSec / 10) % 3
		switch cycle {
		case 0:
			return 2, nil
		case 1:
			return 8, nil
		default:
			return 18, nil
		}
	}

	// Real logic: attempt to connect to sentinel gRPC (implemented in core/sentinel in future)
	// For now, if we cannot reach it, return 0 or fall back to mock cycle
	return 0, fmt.Errorf("gRPC connection to %s failed", gatewayAddr)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SentinelDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&velov1alpha1.SentinelDeployment{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
