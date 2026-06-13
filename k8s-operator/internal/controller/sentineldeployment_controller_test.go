package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	velov1alpha1 "github.com/velo-infra-playground/k8s-operator/api/v1alpha1"
)

func TestReconcile(t *testing.T) {
	// 1. Setup scheme
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = velov1alpha1.AddToScheme(scheme)

	// 2. Create mock worker deployment
	initialReplicas := int32(1)
	mockDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mock-worker-deployment",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &initialReplicas,
		},
		Status: appsv1.DeploymentStatus{
			Replicas: 1,
		},
	}

	// 3. Create SentinelDeployment custom resource
	mockSentinelDep := &velov1alpha1.SentinelDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sentinel-deploy-test",
			Namespace: "default",
		},
		Spec: velov1alpha1.SentinelDeploymentSpec{
			GatewayAddress:   "mock",
			MinReplicas:      2,
			MaxReplicas:      5,
			ScaleThreshold:   5,
			TargetDeployment: "mock-worker-deployment",
		},
	}

	// 4. Initialize fake client with both status subresource and mock objects
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&velov1alpha1.SentinelDeployment{}).
		WithObjects(mockSentinelDep, mockDeployment).
		Build()

	// 5. Setup reconciler
	r := &SentinelDeploymentReconciler{
		Client: cl,
		Scheme: scheme,
	}

	// 6. Run reconciliation
	req := ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name:      "sentinel-deploy-test",
			Namespace: "default",
		},
	}

	res, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Reconciliation failed: %v", err)
	}

	if res.RequeueAfter == 0 {
		t.Error("Expected reconciler to request a requeue")
	}

	// 7. Verify Deployment was scaled
	var updatedDep appsv1.Deployment
	err = cl.Get(context.Background(), client.ObjectKey{Name: "mock-worker-deployment", Namespace: "default"}, &updatedDep)
	if err != nil {
		t.Fatalf("Failed to retrieve updated deployment: %v", err)
	}

	// Verify that replicas is either 2 or 4 (based on mock queue depth cycle & minReplicas=2 bounds)
	replicas := *updatedDep.Spec.Replicas
	if replicas != 2 && replicas != 4 {
		t.Errorf("Expected scaled replicas to be 2 or 4, got %d", replicas)
	}

	// 8. Verify SentinelDeployment Status updates
	var updatedSentinel velov1alpha1.SentinelDeployment
	err = cl.Get(context.Background(), req.NamespacedName, &updatedSentinel)
	if err != nil {
		t.Fatalf("Failed to retrieve custom resource: %v", err)
	}

	activeDepth := updatedSentinel.Status.ActiveQueueDepth
	if activeDepth != 2 && activeDepth != 8 && activeDepth != 18 {
		t.Errorf("Expected status ActiveQueueDepth to be 2, 8, or 18, got %d", activeDepth)
	}

	if updatedSentinel.Status.LastScaleTime == nil {
		t.Error("Expected status LastScaleTime to be populated after scaling event")
	}
}

func TestReconcileDeploymentMissing(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = velov1alpha1.AddToScheme(scheme)

	// Custom resource points to a deployment that does not exist
	mockSentinelDep := &velov1alpha1.SentinelDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sentinel-deploy-test",
			Namespace: "default",
		},
		Spec: velov1alpha1.SentinelDeploymentSpec{
			GatewayAddress:   "mock",
			MinReplicas:      2,
			MaxReplicas:      5,
			ScaleThreshold:   5,
			TargetDeployment: "non-existent-deployment",
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&velov1alpha1.SentinelDeployment{}).
		WithObjects(mockSentinelDep).
		Build()

	r := &SentinelDeploymentReconciler{
		Client: cl,
		Scheme: scheme,
	}

	req := ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name:      "sentinel-deploy-test",
			Namespace: "default",
		},
	}

	// Reconcile should complete successfully without failing (just logging the missing deployment)
	_, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Reconciliation should handle missing deployments gracefully, but failed: %v", err)
	}
}

func TestReconcileSentinelDeploymentMissing(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = velov1alpha1.AddToScheme(scheme)

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &SentinelDeploymentReconciler{
		Client: cl,
		Scheme: scheme,
	}

	req := ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name:      "non-existent-custom-resource",
			Namespace: "default",
		},
	}

	res, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Reconciliation of missing CRD failed: %v", err)
	}
	if res.RequeueAfter != 0 || res.Requeue {
		t.Error("Expected no requeue when Custom Resource is missing")
	}
}

func TestReconcileEnforcesMaxReplicas(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = velov1alpha1.AddToScheme(scheme)

	initialReplicas := int32(10)
	mockDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mock-worker-deployment",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &initialReplicas,
		},
	}

	mockSentinelDep := &velov1alpha1.SentinelDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sentinel-deploy-test",
			Namespace: "default",
		},
		Spec: velov1alpha1.SentinelDeploymentSpec{
			GatewayAddress:   "mock",
			MinReplicas:      1,
			MaxReplicas:      3, // Capped at 3
			ScaleThreshold:   5,
			TargetDeployment: "mock-worker-deployment",
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&velov1alpha1.SentinelDeployment{}).
		WithObjects(mockSentinelDep, mockDeployment).
		Build()

	r := &SentinelDeploymentReconciler{
		Client: cl,
		Scheme: scheme,
	}

	req := ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name:      "sentinel-deploy-test",
			Namespace: "default",
		},
	}

	_, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Reconciliation failed: %v", err)
	}

	var updatedDep appsv1.Deployment
	err = cl.Get(context.Background(), client.ObjectKey{Name: "mock-worker-deployment", Namespace: "default"}, &updatedDep)
	if err != nil {
		t.Fatalf("Failed to retrieve updated deployment: %v", err)
	}

	replicas := *updatedDep.Spec.Replicas
	if replicas > 3 {
		t.Errorf("Expected scaled replicas to be capped at 3, but got %d", replicas)
	}
}

