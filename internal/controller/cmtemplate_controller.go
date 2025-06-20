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

package controller

import (
	"context"

	cachev1alpha1 "github.com/stollenaar/cmstate-injector-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	cmTemplates map[string]cachev1alpha1.CMTemplateSpec
)

// CMTemplateReconciler reconciles a CMTemplate object
type CMTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func init() {
	cmTemplates = make(map[string]cachev1alpha1.CMTemplateSpec)
}

//+kubebuilder:rbac:groups=cache.spicedelver.me,resources=cmtemplates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cache.spicedelver.me,resources=cmtemplates/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cache.spicedelver.me,resources=cmtemplates/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// the CMTemplate object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *CMTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	cmTemplate := &cachev1alpha1.CMTemplate{}
	err := r.Get(ctx, req.NamespacedName, cmTemplate)
	if err != nil {
		// If this is not nil we are already tracking one. So in this case we need to add to the audience
		if apierrors.IsNotFound(err) {
			log.Info("cmtemplate resource was not found. Ignoring, as the object must be deleted")
			delete(cmTemplates, req.NamespacedName.Name)
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get cmtemplate")
		return ctrl.Result{}, err
	}
	cmTemplates[req.NamespacedName.Name] = cmTemplate.Spec
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CMTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("CMTemplateController").
		For(&cachev1alpha1.CMTemplate{}).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For().
		Complete(r)
}
