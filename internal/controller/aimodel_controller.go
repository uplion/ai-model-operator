/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"github.com/go-logr/logr"
	modelv1alpha1 "github.com/uplion/aimodel-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"
	"time"
)

var PulsarUrl = "pulsar://localhost:6650"
var PulsarToken = ""
var ResTopicName = "res-topic"

func init() {
	if v, ok := os.LookupEnv("PULSAR_URL"); ok {
		PulsarUrl = v
	}
	if v, ok := os.LookupEnv("PULSAR_TOKEN"); ok {
		PulsarToken = v
	}
	if v, ok := os.LookupEnv("RES_TOPIC_NAME"); ok {
		ResTopicName = v
	}
}

// AIModelReconciler reconciles a AIModel object
type AIModelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=model.youxam.com,resources=aimodels,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=model.youxam.com,resources=aimodels/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=model.youxam.com,resources=aimodels/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AIModel object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *AIModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the AIModel instance
	aimodel := &modelv1alpha1.AIModel{}
	err := r.Get(ctx, req.NamespacedName, aimodel)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("AIModel resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get AIModel.")
		return ctrl.Result{}, err
	}

	// Define the desired Deployment object
	dep := r.deploymentForAIModel(aimodel)

	// Ensure ServiceAccount, Role, and RoleBinding exist
	sa, err := r.ensureEventServiceAccountRoleAndBinding(ctx, aimodel, dep, logger)
	if err != nil {
		logger.Error(err, "Failed to ensure ServiceAccount, Role and RoleBinding", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		return ctrl.Result{}, err
	}

	// Check if the Deployment already exists
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: dep.Name, Namespace: dep.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			if err := controllerutil.SetControllerReference(aimodel, dep, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}

			// Set initial timestamp for ServiceAccount
			if dep.Spec.Template.ObjectMeta.Annotations == nil {
				dep.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
			}
			dep.Spec.Template.ObjectMeta.Annotations["last-serviceaccount-update"] = sa.CreationTimestamp.Format(time.RFC3339)

			err = r.Create(ctx, dep)
			if err != nil {
				logger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
				return ctrl.Result{}, err
			}
			aimodel.Status.State = "Running"
			aimodel.Status.Message = "AIModel is running successfully"
			err = r.Status().Update(ctx, aimodel)
			if err != nil {
				logger.Error(err, "Failed to update AIModel status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		} else {
			logger.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}
	}

	// Check if ServiceAccount was updated after last recorded update
	lastUpdate, _ := time.Parse(time.RFC3339, found.Spec.Template.ObjectMeta.Annotations["last-serviceaccount-update"])
	needsRestart := sa.CreationTimestamp.After(lastUpdate)

	// Check if we need to update the Deployment
	needsUpdate := !deploymentEqual(found, dep, logger) || needsRestart

	if needsUpdate {
		logger.Info("Updating existing Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		found.Spec = dep.Spec
		found.Labels = dep.Labels

		// If ServiceAccount was updated, force a restart by updating annotations
		if needsRestart {
			if found.Spec.Template.ObjectMeta.Annotations == nil {
				found.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
			}
			found.Spec.Template.ObjectMeta.Annotations["last-serviceaccount-update"] = sa.CreationTimestamp.Format(time.RFC3339)
			found.Spec.Template.ObjectMeta.Annotations["restartedAt"] = time.Now().Format(time.RFC3339)
			logger.Info("Forcing Deployment restart due to ServiceAccount recreation",
				"ServiceAccount.CreationTimestamp", sa.CreationTimestamp,
				"Last.ServiceAccount.Update", lastUpdate)
		}

		err = r.Update(ctx, found)
		if err != nil {
			logger.Error(err, "Failed to update Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}

		aimodel.Status.State = "Running"
		aimodel.Status.Message = "AIModel is running successfully"
		err = r.Status().Update(ctx, aimodel)
		if err != nil {
			logger.Error(err, "Failed to update AIModel status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

func (r *AIModelReconciler) deploymentForAIModel(m *modelv1alpha1.AIModel) *appsv1.Deployment {
	labels := m.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	labels["aimodel-internel-selector"] = m.Name

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name + "-deployment",
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: m.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"aimodel-internel-selector": m.Name,
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: v1.PodSpec{
					ServiceAccountName: m.Name + "-deployment-sa",
					Containers: []v1.Container{
						{
							Name:  m.Name,
							Image: m.Spec.Image,
							Env: []v1.EnvVar{
								{
									Name:  "NODE_TYPE",
									Value: m.Spec.Type,
								},
								{
									Name:  "MODEL_NAME",
									Value: m.Spec.Model,
								},
								{
									Name:  "API_URL",
									Value: m.Spec.BaseURL,
								},
								{
									Name:  "API_KEY",
									Value: m.Spec.APIKey,
								},
								{
									Name:  "MAX_PROCESS_NUM",
									Value: "128",
								},
								{
									Name:  "PULSAR_URL",
									Value: PulsarUrl,
								},
								{
									Name:  "PULSAR_TOKEN",
									Value: PulsarToken,
								},
								{
									Name:  "RES_TOPIC_NAME",
									Value: ResTopicName,
								},
								{
									Name:  "AIMODEL_NAME",
									Value: m.Name,
								},
								{
									Name: "AIMODEL_NAMESPACE",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if m.Spec.MaxProcessNum != nil {
		dep.Spec.Template.Spec.Containers[0].Env[4].Value = strconv.Itoa(int(*m.Spec.MaxProcessNum))
	}

	return dep
}

func (r *AIModelReconciler) ensureEventServiceAccountRoleAndBinding(ctx context.Context, aimodel *modelv1alpha1.AIModel, dep *appsv1.Deployment, logger logr.Logger) (*corev1.ServiceAccount, error) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dep.Name + "-sa",
			Namespace: dep.Namespace,
		},
	}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dep.Name + "-event-role",
			Namespace: dep.Namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "patch", "update"},
			},
		},
	}
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dep.Name + "-event-rolebinding",
			Namespace: dep.Namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: dep.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     role.Name,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	// Set AIModel instance as the owner and controller
	if err := controllerutil.SetControllerReference(aimodel, sa, r.Scheme); err != nil {
		return nil, err
	}
	if err := controllerutil.SetControllerReference(aimodel, role, r.Scheme); err != nil {
		return nil, err
	}
	if err := controllerutil.SetControllerReference(aimodel, roleBinding, r.Scheme); err != nil {
		return nil, err
	}

	// Ensure ServiceAccount
	err := r.Create(ctx, sa)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// If it exists, update it
			if err := r.Update(ctx, sa); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else {
		logger.Info("ServiceAccount created", "ServiceAccount.Namespace", sa.Namespace, "ServiceAccount.Name", sa.Name)
	}

	// Ensure Role
	err = r.Create(ctx, role)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// If it exists, update it
			if err := r.Update(ctx, role); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else {
		logger.Info("Role created", "Role.Namespace", role.Namespace, "Role.Name", role.Name)
	}

	// Ensure RoleBinding
	err = r.Create(ctx, roleBinding)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// If it exists, update it
			if err := r.Update(ctx, roleBinding); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else {
		logger.Info("RoleBinding created", "RoleBinding.Namespace", roleBinding.Namespace, "RoleBinding.Name", roleBinding.Name)
	}

	return sa, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AIModelReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&modelv1alpha1.AIModel{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(r)
}

func deploymentEqual(a, b *appsv1.Deployment, logger logr.Logger) bool {
	// Compare the relevant fields to determine if they are equal

	if *a.Spec.Replicas != *b.Spec.Replicas {
		logger.Info("Replicas changed", "old", *a.Spec.Replicas, "new", *b.Spec.Replicas)
		return false
	}
	if !equalMaps(a.Labels, b.Labels) {
		logger.Info("Labels changed", "old", a.Labels, "new", b.Labels)
		return false
	}
	if !equalMaps(a.Spec.Template.Labels, b.Spec.Template.Labels) {
		logger.Info("Template Labels changed", "old", a.Spec.Template.Labels, "new", b.Spec.Template.Labels)
		return false
	}
	if a.Spec.Template.Spec.Containers[0].Image != b.Spec.Template.Spec.Containers[0].Image {
		logger.Info("Image changed", "old", a.Spec.Template.Spec.Containers[0].Image, "new", b.Spec.Template.Spec.Containers[0].Image)
		return false
	}
	if !equalEnv(a.Spec.Template.Spec.Containers[0].Env, b.Spec.Template.Spec.Containers[0].Env) {
		logger.Info("Env changed", "old", a.Spec.Template.Spec.Containers[0].Env, "new", b.Spec.Template.Spec.Containers[0].Env)
		return false
	}
	return true
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func equalEnv(a, b []v1.EnvVar) bool {
	if len(a) != len(b) {
		return false
	}
	envMap := make(map[string]string, len(a))
	for _, e := range a {
		envMap[e.Name] = e.Value
	}
	for _, e := range b {
		if envMap[e.Name] != e.Value {
			return false
		}
	}
	return true
}
