/*
Copyright 2023.

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

package controllers

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cachev1alpha1 "github.com/stollenaar/cmstate-injector-operator/api/v1alpha1"
)

// Definitions to manage status conditions
const (
	// typeAvailableCMState represents the status of the ConfigMap reconciliation
	typeAvailableCMState = "Available"
)

var (
	//go:embed vault-agent.hcl.tpl
	agentTemplate string
	ownNamespace  string
)

// TODO: this is ugly and needs to be better
func init() {
	d, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		panic(err)
	}
	ownNamespace = string(d)
}

// CMStateReconciler reconciles a CMState object
type CMStateReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=cache.spices.dev,resources=cmstates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cache.spices.dev,resources=cmstates/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cache.spices.dev,resources=cmstates/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *CMStateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	cmState := &cachev1alpha1.CMState{}
	err := r.Get(ctx, req.NamespacedName, cmState)
	if err != nil {
		// If this is not nil we are already tracking one. So in this case we need to add to the audience
		if apierrors.IsNotFound(err) {
			log.Info("cmstate resource was not found. Ignoring, as the object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get cmstate")
		return ctrl.Result{}, err
	}

	// Check if the CmState instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isCmStateMarkedToBeDeleted := cmState.GetDeletionTimestamp() != nil
	if isCmStateMarkedToBeDeleted {
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: cmState.Spec.Target,
			},
		}
		r.Delete(ctx, cm)
		return ctrl.Result{}, nil
	}

	found := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: cmState.Spec.Target, Namespace: cmState.Namespace}, found)
	if cmState.Spec.Target == "" {
		cm, err := r.configMapForCMState(cmState, ctx)
		if err != nil {
			log.Error(err, "Failed to define new Configmap resource for CMState")

			// The following implementation will update the status
			meta.SetStatusCondition(&cmState.Status.Conditions, metav1.Condition{Type: typeAvailableCMState,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Configmap for the custom resource (%s): (%s)", cmState.Name, err)})

			if err := r.Status().Update(ctx, cmState); err != nil {
				log.Error(err, "Failed to update CMState status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
		log.Info("Creating a new ConfigMap", "ConfigMap.Namespace", cm.Namespace, "ConfigMap.Name", cm.Name)
		if err = r.Create(ctx, cm); err != nil {
			log.Error(err, "Failed to create new ConfigMap", "ConfigMap.Namespace", cm.Namespace, "ConfigMap.Name", cm.Name)
			return ctrl.Result{}, err
		}
		cmState.Spec.Target = cm.GetName()
		err = r.Patch(ctx, cmState, client.Apply)
		if err != nil {
			log.Error(err, "Failed to update CMState Audience")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get ConfigMap")
		return ctrl.Result{}, err
	}

	if len(cmState.Spec.Audience) == 0 {
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmState.Spec.Target,
				Namespace: cmState.GetNamespace(),
			},
		}
		err = r.Delete(ctx, cm)
		if err != nil {
			log.Error(err, "Failed to delete tracked ConfigMap")
		}
		err = r.Delete(ctx, cmState)
		if err != nil {
			log.Error(err, "Failed to delete CMState")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CMStateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.CMState{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}

func eventFilter() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			annotations := e.Object.GetAnnotations()
			vi := annotations["vault.hashicorp.com/agent-inject"]
			vir := annotations["vault.hashicorp.com/agent-internal-role"]
			vawsr := annotations["vault.hashicorp.com/agent-aws-role"]
			return vi == "true" && vir != "" && vawsr != ""
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			annotations := e.Object.GetAnnotations()
			vi := annotations["vault.hashicorp.com/agent-inject"]
			vir := annotations["vault.hashicorp.com/agent-internal-role"]
			vawsr := annotations["vault.hashicorp.com/agent-aws-role"]
			return vi == "true" && vir != "" && vawsr != ""
		},
	}
}

func (r *CMStateReconciler) findObjectsForCMState(pod client.Object) []reconcile.Request {
	ctx := context.TODO()
	log := log.FromContext(ctx)

	// Creating a selector based on the current pod
	annotations := pod.GetAnnotations()
	name := generateName(annotations)

	cmState := &cachev1alpha1.CMState{}

	// If no CMState exists yet we create and add it to the reconcile loop
	err := r.Get(ctx, types.NamespacedName{Namespace: pod.GetNamespace(), Name: name}, cmState)
	if err != nil && apierrors.IsNotFound(err) {
		cm := generateCMState(pod)
		log.Info("Generating cmState", "CMState", cm)
		err = r.Create(ctx, cm)
		if err != nil {
			log.Error(err, "Error creating cmState")
			return []reconcile.Request{}
		}
		return []reconcile.Request{}
	}
	var requests []reconcile.Request

	// Check if the pod is being deleted or being created
	isPodToBeDeleted := pod.GetDeletionTimestamp() != nil
	if isPodToBeDeleted {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: cmState.GetNamespace(),
				Name:      cmState.GetName(),
			},
		})
	} else {
		// Pod is being created and an existing CMState exists so we add it to the audience
		if !hasAudience(pod.GetName(), cmState.Spec.Audience) {
			cmState.Spec.Audience = append(cmState.Spec.Audience, cachev1alpha1.CMAudience{
				Kind: "Pod",
				Name: pod.GetName(),
			})
			err = r.Patch(ctx, cmState, client.Merge)
			if err != nil {
				log.Error(err, "Error adding audience member to cmState")
				return requests
			}
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: cmState.GetNamespace(),
					Name:      cmState.GetName(),
				},
			})
		}
	}
	return requests
}

func hasAudience(podName string, audience []cachev1alpha1.CMAudience) bool {
	for _, aud := range audience {
		if aud.Name == podName {
			return true
		}
	}
	return false
}

// configMapForCMState returns a CMState Deployment object
func (r *CMStateReconciler) configMapForCMState(
	cmstate *cachev1alpha1.CMState, ctx context.Context) (*corev1.ConfigMap, error) {
	cmTemplate := &cachev1alpha1.CMTemplate{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: ownNamespace,
		Name:      cmstate.Spec.CMTemplate,
	}, cmTemplate)
	if err != nil {
		return nil, err
	}

	labels := cmstate.GetLabels()

	data := make(map[string]string)

	for key, template :=range cmTemplate.Spec.Template.CMTemplate {
		for annotation, templateKey := range cmTemplate.Spec.Template.AnnotationReplace {
			template = strings.ReplaceAll(template,templateKey, labels[annotation] )
		} 
		data[key] = template
	}
	// configReplace := strings.NewReplacer("${exit_after_auth}", "false", "${internal_role_name}", labels["internal-role"], "${aws_role_name}", labels["aws-role"])
	// configInitReplace := strings.NewReplacer("${exit_after_auth}", "true", "${internal_role_name}", labels["internal-role"], "${aws_role_name}", labels["aws-role"])

	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmstate.Name,
			Namespace: cmstate.GetNamespace(),
		},
		Data: data,
		// Data: map[string]string{
		// 	"config.hcl":      configReplace.Replace(agentTemplate),
		// 	"config-init.hcl": configInitReplace.Replace(agentTemplate),
		// },
	}, nil
}

// Generating a CMState used for later
func generateCMState(pod client.Object) *cachev1alpha1.CMState {
	annotations := pod.GetAnnotations()

	return &cachev1alpha1.CMState{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cache.spices.dev/v1alpha1",
			Kind:       "CMState",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateName(annotations),
			Namespace: pod.GetNamespace(),
			Labels: map[string]string{
				"aws-role":      annotations["vault.hashicorp.com/agent-aws-role"],
				"internal-role": annotations["vault.hashicorp.com/agent-internal-role"],
			},
		},
		Spec: cachev1alpha1.CMStateSpec{
			Audience: []cachev1alpha1.CMAudience{
				{
					Kind: "Pod",
					Name: pod.GetName(),
				},
			},
		},
	}
}

func generateName(annotations map[string]string) string {
	return strings.ToLower(strings.ReplaceAll(fmt.Sprintf("cmstate-%s-%s", annotations["vault.hashicorp.com/agent-internal-role"], annotations["vault.hashicorp.com/agent-aws-role"]), "_", "-"))
}
