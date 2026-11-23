package kubernetes

import (
	"context"
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"internal-developer-platform/src/infra/clients/resilience"
	apperrors "internal-developer-platform/src/shared/errors"
)

type KubernetesClient struct {
	clientset kubernetes.Interface
	executor  *resilience.ResilienceExecutor
}

func NewKubernetesClient(clientset kubernetes.Interface, config resilience.ResilienceConfig) *KubernetesClient {
	return &KubernetesClient{
		clientset: clientset,
		executor:  resilience.NewResilienceExecutor(config),
	}
}

func NewKubernetesClientFromConfig(kubeconfigPath string, masterURL string, config resilience.ResilienceConfig) (*KubernetesClient, error) {
	var restConfig *rest.Config
	var err error

	if kubeconfigPath == "" && masterURL == "" {
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			if home := homedir.HomeDir(); home != "" {
				defaultPath := filepath.Join(home, ".kube", "config")
				restConfig, err = clientcmd.BuildConfigFromFlags("", defaultPath)
			}
		}
	} else {
		restConfig, err = clientcmd.BuildConfigFromFlags(masterURL, kubeconfigPath)
	}

	if err != nil {
		return nil, apperrors.NewDownstreamUnavailableError("failed to construct kubernetes rest configuration: " + err.Error())
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, apperrors.NewDownstreamUnavailableError("failed to initialize kubernetes clientset: " + err.Error())
	}

	return NewKubernetesClient(clientset, config), nil
}

func (c *KubernetesClient) Clientset() kubernetes.Interface {
	return c.clientset
}

func (c *KubernetesClient) Resilience() *resilience.ResilienceExecutor {
	return c.executor
}

func (c *KubernetesClient) CreateOrUpdateNamespace(ctx context.Context, name string, labels map[string]string) (*corev1.Namespace, error) {
	var result *corev1.Namespace
	err := c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		existing, getErr := c.clientset.CoreV1().Namespaces().Get(opCtx, name, metav1.GetOptions{})
		if getErr != nil {
			if k8serrors.IsNotFound(getErr) {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   name,
						Labels: labels,
					},
				}
				created, createErr := c.clientset.CoreV1().Namespaces().Create(opCtx, ns, metav1.CreateOptions{})
				if createErr != nil {
					return c.mapKubernetesError(createErr)
				}
				result = created
				return nil
			}
			return c.mapKubernetesError(getErr)
		}

		if existing.Labels == nil {
			existing.Labels = make(map[string]string)
		}
		for k, v := range labels {
			existing.Labels[k] = v
		}

		updated, updateErr := c.clientset.CoreV1().Namespaces().Update(opCtx, existing, metav1.UpdateOptions{})
		if updateErr != nil {
			return c.mapKubernetesError(updateErr)
		}
		result = updated
		return nil
	})
	return result, err
}

func (c *KubernetesClient) GetResourceQuota(ctx context.Context, namespace string, name string) (*corev1.ResourceQuota, error) {
	var result *corev1.ResourceQuota
	err := c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		quota, getErr := c.clientset.CoreV1().ResourceQuotas(namespace).Get(opCtx, name, metav1.GetOptions{})
		if getErr != nil {
			return c.mapKubernetesError(getErr)
		}
		result = quota
		return nil
	})
	return result, err
}

func (c *KubernetesClient) CreateOrUpdateResourceQuota(ctx context.Context, namespace string, quota *corev1.ResourceQuota) (*corev1.ResourceQuota, error) {
	var result *corev1.ResourceQuota
	err := c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		existing, getErr := c.clientset.CoreV1().ResourceQuotas(namespace).Get(opCtx, quota.Name, metav1.GetOptions{})
		if getErr != nil {
			if k8serrors.IsNotFound(getErr) {
				created, createErr := c.clientset.CoreV1().ResourceQuotas(namespace).Create(opCtx, quota, metav1.CreateOptions{})
				if createErr != nil {
					return c.mapKubernetesError(createErr)
				}
				result = created
				return nil
			}
			return c.mapKubernetesError(getErr)
		}

		existing.Spec = quota.Spec
		existing.Labels = quota.Labels
		updated, updateErr := c.clientset.CoreV1().ResourceQuotas(namespace).Update(opCtx, existing, metav1.UpdateOptions{})
		if updateErr != nil {
			return c.mapKubernetesError(updateErr)
		}
		result = updated
		return nil
	})
	return result, err
}

func (c *KubernetesClient) CreateOrUpdateDeployment(ctx context.Context, namespace string, deploy *appsv1.Deployment) (*appsv1.Deployment, error) {
	var result *appsv1.Deployment
	err := c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		existing, getErr := c.clientset.AppsV1().Deployments(namespace).Get(opCtx, deploy.Name, metav1.GetOptions{})
		if getErr != nil {
			if k8serrors.IsNotFound(getErr) {
				created, createErr := c.clientset.AppsV1().Deployments(namespace).Create(opCtx, deploy, metav1.CreateOptions{})
				if createErr != nil {
					return c.mapKubernetesError(createErr)
				}
				result = created
				return nil
			}
			return c.mapKubernetesError(getErr)
		}

		existing.Spec = deploy.Spec
		existing.Labels = deploy.Labels
		updated, updateErr := c.clientset.AppsV1().Deployments(namespace).Update(opCtx, existing, metav1.UpdateOptions{})
		if updateErr != nil {
			return c.mapKubernetesError(updateErr)
		}
		result = updated
		return nil
	})
	return result, err
}

func (c *KubernetesClient) GetDeployment(ctx context.Context, namespace string, name string) (*appsv1.Deployment, error) {
	var result *appsv1.Deployment
	err := c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		deploy, getErr := c.clientset.AppsV1().Deployments(namespace).Get(opCtx, name, metav1.GetOptions{})
		if getErr != nil {
			return c.mapKubernetesError(getErr)
		}
		result = deploy
		return nil
	})
	return result, err
}

func (c *KubernetesClient) CreateOrUpdatePDB(ctx context.Context, namespace string, pdb *policyv1.PodDisruptionBudget) (*policyv1.PodDisruptionBudget, error) {
	var result *policyv1.PodDisruptionBudget
	err := c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		existing, getErr := c.clientset.PolicyV1().PodDisruptionBudgets(namespace).Get(opCtx, pdb.Name, metav1.GetOptions{})
		if getErr != nil {
			if k8serrors.IsNotFound(getErr) {
				created, createErr := c.clientset.PolicyV1().PodDisruptionBudgets(namespace).Create(opCtx, pdb, metav1.CreateOptions{})
				if createErr != nil {
					return c.mapKubernetesError(createErr)
				}
				result = created
				return nil
			}
			return c.mapKubernetesError(getErr)
		}

		existing.Spec = pdb.Spec
		existing.Labels = pdb.Labels
		updated, updateErr := c.clientset.PolicyV1().PodDisruptionBudgets(namespace).Update(opCtx, existing, metav1.UpdateOptions{})
		if updateErr != nil {
			return c.mapKubernetesError(updateErr)
		}
		result = updated
		return nil
	})
	return result, err
}

func (c *KubernetesClient) GetDeploymentReadyReplicas(ctx context.Context, namespace string, name string) (int32, error) {
	var ready int32
	err := c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		deploy, getErr := c.clientset.AppsV1().Deployments(namespace).Get(opCtx, name, metav1.GetOptions{})
		if getErr != nil {
			return c.mapKubernetesError(getErr)
		}
		ready = deploy.Status.ReadyReplicas
		return nil
	})
	return ready, err
}

func (c *KubernetesClient) mapKubernetesError(err error) error {
	if err == nil {
		return nil
	}

	if k8serrors.IsNotFound(err) {
		return apperrors.NewNotFoundError(err.Error())
	}
	if k8serrors.IsConflict(err) {
		return apperrors.NewConflictError(err.Error())
	}
	if k8serrors.IsUnauthorized(err) {
		return apperrors.NewUnauthorizedError(err.Error())
	}
	if k8serrors.IsForbidden(err) {
		return apperrors.NewForbiddenError(err.Error())
	}
	if k8serrors.IsServerTimeout(err) || k8serrors.IsServiceUnavailable(err) || k8serrors.IsTooManyRequests(err) {
		return apperrors.NewDownstreamUnavailableError(err.Error())
	}

	return apperrors.NewDownstreamUnavailableError(err.Error())
}
