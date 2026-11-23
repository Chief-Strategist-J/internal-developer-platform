package kubernetes

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"

	"internal-developer-platform/src/infra/clients/resilience"
)

func TestKubernetesClientNamespaceLifecycle(t *testing.T) {
	fakeClientset := fake.NewSimpleClientset()
	cfg := resilience.DefaultResilienceConfig()
	client := NewKubernetesClient(fakeClientset, cfg)

	ctx := context.Background()
	labels := map[string]string{
		"environment": "production",
		"managed-by":  "idp",
	}

	ns, err := client.CreateOrUpdateNamespace(ctx, "tenant-alpha", labels)
	if err != nil {
		t.Fatalf("expected namespace creation to succeed: %v", err)
	}
	if ns.Name != "tenant-alpha" {
		t.Fatalf("expected namespace name 'tenant-alpha', got %s", ns.Name)
	}

	labels["tier"] = "frontend"
	updatedNs, err := client.CreateOrUpdateNamespace(ctx, "tenant-alpha", labels)
	if err != nil {
		t.Fatalf("expected namespace update to succeed: %v", err)
	}
	if updatedNs.Labels["tier"] != "frontend" {
		t.Fatalf("expected label tier=frontend, got %v", updatedNs.Labels)
	}
}

func TestKubernetesClientResourceQuota(t *testing.T) {
	fakeClientset := fake.NewSimpleClientset()
	cfg := resilience.DefaultResilienceConfig()
	client := NewKubernetesClient(fakeClientset, cfg)

	ctx := context.Background()
	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "compute-quota",
			Namespace: "tenant-alpha",
		},
	}

	created, err := client.CreateOrUpdateResourceQuota(ctx, "tenant-alpha", quota)
	if err != nil {
		t.Fatalf("expected quota create to succeed: %v", err)
	}
	if created.Name != "compute-quota" {
		t.Fatalf("unexpected quota name: %s", created.Name)
	}

	retrieved, err := client.GetResourceQuota(ctx, "tenant-alpha", "compute-quota")
	if err != nil {
		t.Fatalf("expected quota get to succeed: %v", err)
	}
	if retrieved.Name != "compute-quota" {
		t.Fatalf("expected compute-quota, got %s", retrieved.Name)
	}
}

func TestKubernetesClientDeploymentAndPDB(t *testing.T) {
	fakeClientset := fake.NewSimpleClientset()
	cfg := resilience.DefaultResilienceConfig()
	client := NewKubernetesClient(fakeClientset, cfg)

	ctx := context.Background()
	replicas := int32(3)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-service",
			Namespace: "tenant-alpha",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
	}

	created, err := client.CreateOrUpdateDeployment(ctx, "tenant-alpha", deploy)
	if err != nil {
		t.Fatalf("expected deployment creation to succeed: %v", err)
	}
	if *created.Spec.Replicas != 3 {
		t.Fatalf("expected 3 replicas, got %d", *created.Spec.Replicas)
	}

	minAvailable := intstr.FromInt(1)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-service-pdb",
			Namespace: "tenant-alpha",
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
		},
	}

	createdPDB, err := client.CreateOrUpdatePDB(ctx, "tenant-alpha", pdb)
	if err != nil {
		t.Fatalf("expected pdb creation to succeed: %v", err)
	}
	if createdPDB.Name != "web-service-pdb" {
		t.Fatalf("expected web-service-pdb, got %s", createdPDB.Name)
	}

	ready, err := client.GetDeploymentReadyReplicas(ctx, "tenant-alpha", "web-service")
	if err != nil {
		t.Fatalf("expected read of ready replicas: %v", err)
	}
	if ready != 0 {
		t.Fatalf("expected 0 ready replicas on newly created deployment, got %d", ready)
	}
}

func TestKubernetesClientResilienceIntegration(t *testing.T) {
	fakeClientset := fake.NewSimpleClientset()
	cfg := resilience.DefaultResilienceConfig()
	cfg.MinDeadlineMargin = 50 * time.Millisecond
	cfg.ConnectTimeout = 100 * time.Millisecond
	client := NewKubernetesClient(fakeClientset, cfg)

	expiredCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := client.GetResourceQuota(expiredCtx, "tenant-alpha", "compute-quota")
	if err == nil {
		t.Fatalf("expected error when context deadline budget is insufficient")
	}
}
