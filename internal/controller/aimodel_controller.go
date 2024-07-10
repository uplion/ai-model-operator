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
)

var MaxProcessNum = "128"
var PulsarUrl = "pulsar://localhost:6650"
var PulsarToken = ""
var ResTopicName = "res-topic"

func init() {
	if v, ok := os.LookupEnv("MAX_PROCESS_NUM"); ok {
		MaxProcessNum = v
	}
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
			// Request object not found, could have been deleted after reconcile request.
			// Return and don't requeue
			logger.Info("AIModel resource not found. Ignoring since object must be deleted.")

			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get AIModel.")
		return ctrl.Result{}, err
	}

	// Define the desired Deployment object
	dep := r.deploymentForAIModel(aimodel)

	// Check if the Deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: dep.Name, Namespace: dep.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {

		// Create ServiceAccount, Role and RoleBinding for the Deployment to create events
		err = r.createEventServiceAccountRoleAndBinding(ctx, aimodel, dep, logger)
		if err != nil {
			logger.Error(err, "Failed to create ServiceAccount, Role and RoleBinding", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}

		logger.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		// Set AIModel instance as the owner and controller
		if err := controllerutil.SetControllerReference(aimodel, dep, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}

		err = r.Create(ctx, dep)
		if err != nil {
			logger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}

		// Update the AIModel status
		aimodel.Status.State = "Running"
		aimodel.Status.Message = "AIModel is running successfully"
		err = r.Status().Update(ctx, aimodel)
		if err != nil {
			logger.Error(err, "Failed to update AIModel status")
			return ctrl.Result{}, err
		}

		// Deployment created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		logger.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	// Check for any changes in AIModel spec or labels
	if !deploymentEqual(found, dep, logger) {
		logger.Info("Updating existing Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		found.Spec = dep.Spec
		found.Labels = dep.Labels
		err = r.Update(ctx, found)
		if err != nil {
			logger.Error(err, "Failed to update Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}

		// Update the AIModel status
		aimodel.Status.State = "Running"
		aimodel.Status.Message = "AIModel is running successfully"
		err = r.Status().Update(ctx, aimodel)
		if err != nil {
			logger.Error(err, "Failed to update AIModel status")
			return ctrl.Result{}, err
		}

		// Deployment updated successfully - return and requeue
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

	return &appsv1.Deployment{
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
									Value: MaxProcessNum,
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
}

func (r *AIModelReconciler) createEventServiceAccountRoleAndBinding(ctx context.Context, aimodel *modelv1alpha1.AIModel, dep *appsv1.Deployment, logger logr.Logger) error {
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
		return err
	}
	if err := controllerutil.SetControllerReference(aimodel, role, r.Scheme); err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(aimodel, roleBinding, r.Scheme); err != nil {
		return err
	}

	// Create ServiceAccount
	err := r.Create(ctx, sa)
	if !errors.IsAlreadyExists(err) {
		if err != nil {
			return err
		}
		logger.Info("ServiceAccount created", "ServiceAccount.Namespace", sa.Namespace, "ServiceAccount.Name", sa.Name)
	}

	// Create Role
	err = r.Create(ctx, role)
	if !errors.IsAlreadyExists(err) {
		if err != nil {
			return err
		}
		logger.Info("Role created", "Role.Namespace", role.Namespace, "Role.Name", role.Name)
	}

	// Create RoleBinding
	err = r.Create(ctx, roleBinding)
	if !errors.IsAlreadyExists(err) {
		if err != nil {
			return err
		}
		logger.Info("RoleBinding created", "RoleBinding.Namespace", roleBinding.Namespace, "RoleBinding.Name", roleBinding.Name)

	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AIModelReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&modelv1alpha1.AIModel{}).
		// The following code is used to listen to the Deployment, ServiceAccount, Role and RoleBinding,
		// which is not necessary for the current scenario
		//Owns(&appsv1.Deployment{}).
		//Owns(&corev1.ServiceAccount{}).
		//Owns(&rbacv1.Role{}).
		//Owns(&rbacv1.RoleBinding{}).
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
