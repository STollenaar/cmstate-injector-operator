package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pkg/errors"
	cachev1alpha1 "github.com/stollenaar/cmstate-injector-operator/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"

	v1admission "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,failurePolicy=ignore,sideEffects=None,groups="",resources=pods,verbs=create;delete,versions=v1,name=cmstate-operator-webhook.spicedelver.me,admissionReviewVersions=v1

type PatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

type cmStateCreator struct {
	Client  client.Client
	decoder *admission.Decoder
}

func CMStateCreator(mgr ctrl.Manager) error {
	hookServer := mgr.GetWebhookServer()
	hookServer.Register("/mutate-v1-pod", &webhook.Admission{Handler: &cmStateCreator{Client: mgr.GetClient()}})
	return nil
}

// cmStateCreator creates the cmstate if needed or patches the audience.
func (hook *cmStateCreator) Handle(ctx context.Context, req admission.Request) admission.Response {
	resp, err := hook.handleInner(ctx, req)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return *resp
}

func (hook *cmStateCreator) handleInner(ctx context.Context, req admission.Request) (*admission.Response, error) {
	log := ctrl.Log.WithName("webhooks").WithName("CMStateCreator")

	var err error
	pod := &corev1.Pod{}
	switch req.Operation {
	case v1admission.Create:
		err = (*hook.decoder).Decode(req, pod)
	case v1admission.Delete:
		err = (*hook.decoder).DecodeRaw(req.OldObject, pod)
	default:
		resp := admission.Allowed("skipping cmstate check due to bad operation")
		return &resp, nil
	}
	if err != nil {
		log.Error(err, "Error decoding request into Pod")
		return nil, errors.Wrap(err, "error decoding request into Pod")
	}

	cmState := &cachev1alpha1.CMState{}
	cmTemplate := &cachev1alpha1.CMTemplate{}
	if pod.Annotations["cache.spicedelver.me/cmtemplate"] != "" {

		crdName := generateName(pod.Annotations["cache.spicedelver.me/cmtemplate"])
		err = hook.Client.Get(
			ctx,
			types.NamespacedName{
				Namespace: pod.Namespace,
				Name:      crdName,
			},
			cmState,
		)

		if err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "fetching cmstate has resulted in an error")
			return nil, errors.Wrap(err, "fetching cmstate has resulted in an error")
		}
		err = hook.Client.Get(
			ctx,
			types.NamespacedName{
				Name: pod.Annotations["cache.spicedelver.me/cmtemplate"],
			},
			cmTemplate,
		)

		if err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "fetching cmtemplate has resulted in an error")
			return nil, errors.Wrap(err, "fetching cmtemplate has resulted in an error")
		} else if err != nil {
			log.Error(err, "fetching cmtemplate has resulted in an error")
			return nil, errors.Wrap(err, "fetching cmtemplate has resulted in an error")
		}

		switch req.Operation {
		case v1admission.Create:
			return hook.handlePodCreate(req, cmState, cmTemplate, pod, ctx)
		case v1admission.Delete:
			return hook.handlePodDelete(cmState, pod, ctx)
		}
	}
	resp := admission.Allowed("skipping cmstate check due to missing annotation")
	return &resp, nil
}

func (hook *cmStateCreator) handlePodDelete(cmState *cachev1alpha1.CMState, pod *corev1.Pod, ctx context.Context) (*admission.Response, error) {
	if cmState.Name == "" {
		resp := admission.Allowed("skipping cmstate patch due to missing cmstate")
		return &resp, nil
	}
	podName := pod.GetName()
	if pod.GetGenerateName() != "" {
		podName = pod.GetGenerateName()
	}

	if len(pod.GetOwnerReferences()) > 0 && hook.checkOwners(pod, ctx) {
		resp := admission.Allowed("skipping cmstate patch due to pod being kept around")
		return &resp, nil
	}

	index := findIndex(cmState.Spec.Audience, podName)
	if index == -1 {
		resp := admission.Allowed("skipping cmstate patch due to pod not in audience")
		return &resp, nil
	}
	cmState.Spec.Audience = append(cmState.Spec.Audience[:index], cmState.Spec.Audience[index+1:]...)

	err := hook.Client.Patch(ctx, cmState, client.Merge)
	if err != nil {
		resp := admission.Denied("patching cmstate has resulted in an error")
		return &resp, err
	}

	resp := admission.Allowed("cmstate has been patched, no need to mutate pod")
	return &resp, nil
}

func (hook *cmStateCreator) handlePodCreate(req admission.Request, cmState *cachev1alpha1.CMState, cmTemplate *cachev1alpha1.CMTemplate, pod *corev1.Pod, ctx context.Context) (*admission.Response, error) {
	if cmState.Name == "" {
		// create the cmstate
		cmState = generateCMState(cmTemplate, pod)

		err := hook.Client.Create(ctx, cmState)

		if err != nil {
			resp := admission.Denied("creating cmstate has resulted in an error")
			return &resp, err
		}
	}

	pod.Annotations[cmTemplate.Spec.Template.TargetAnnotation] = cmState.Name

	pData, err := json.Marshal(pod)
	if err != nil {
		return nil, errors.Wrap(err, "error encoding response object")
	}

	resp := admission.PatchResponseFromRaw(req.Object.Raw, pData)
	return &resp, nil
}

// Generating a CMState used for later
func generateCMState(cmTemplate *cachev1alpha1.CMTemplate, pod *corev1.Pod) *cachev1alpha1.CMState {
	annotations := pod.GetAnnotations()

	labels := make(map[string]string)
	for annotation := range cmTemplate.Spec.Template.AnnotationReplace {
		labels[annotation] = annotations[annotation]
	}

	podName := pod.GetName()
	if podName == "" {
		podName = pod.GetGenerateName()
	}
	return &cachev1alpha1.CMState{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cache.spicedelver.me/v1alpha1",
			Kind:       "CMState",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateName(cmTemplate.Name),
			Namespace: pod.GetNamespace(),
			Labels:    labels,
		},
		Spec: cachev1alpha1.CMStateSpec{
			Audience: []cachev1alpha1.CMAudience{
				{
					Kind: "Pod",
					Name: podName,
				},
			},
			CMTemplate: cmTemplate.Name,
		},
	}
}

func generateName(cmTemplateName string) string {
	return strings.ToLower(strings.ReplaceAll(fmt.Sprintf("cmstate-%s", cmTemplateName), "_", "-"))
}

func findIndex(slice []cachev1alpha1.CMAudience, name string) int {
	for i, aud := range slice {
		if aud.Name == name {
			return i
		}
	}
	return -1
}

// cmStateCreator implements admission.DecoderInjector.
// A decoder will be automatically injected.

// InjectDecoder injects the decoder.
func (hook *cmStateCreator) InjectDecoder(d *admission.Decoder) error {
	hook.decoder = d
	return nil
}

func (hook *cmStateCreator) checkOwners(pod *corev1.Pod, ctx context.Context) bool {
	for _, owner := range pod.GetOwnerReferences() {
		if !hook.checkOwner(owner, pod, ctx) {
			return false
		}
	}
	return true
}

func (hook *cmStateCreator) checkOwner(owner metav1.OwnerReference, pod *corev1.Pod, ctx context.Context) bool {
	switch owner.Kind {
	case "Deployment":
		ownerObject := &appsv1.Deployment{}
		err := hook.Client.Get(
			ctx,
			types.NamespacedName{
				Namespace: pod.Namespace,
				Name:      owner.Name,
			},
			ownerObject,
		)
		if err != nil {
			fmt.Println(err)
			return false
		}
		return *ownerObject.Spec.Replicas != 0
	case "DaemonSet":
		ownerObject := &appsv1.DaemonSet{}
		err := hook.Client.Get(
			ctx,
			types.NamespacedName{
				Namespace: pod.Namespace,
				Name:      owner.Name,
			},
			ownerObject,
		)
		if err != nil {
			fmt.Println(err)
			return false
		}
		return true
	case "StatefulSet":
		ownerObject := &appsv1.StatefulSet{}
		err := hook.Client.Get(
			ctx,
			types.NamespacedName{
				Namespace: pod.Namespace,
				Name:      owner.Name,
			},
			ownerObject,
		)
		if err != nil {
			fmt.Println(err)
			return false
		}
		return *ownerObject.Spec.Replicas != 0
	case "ReplicaSet":
		ownerObject := &appsv1.ReplicaSet{}
		err := hook.Client.Get(
			ctx,
			types.NamespacedName{
				Namespace: pod.Namespace,
				Name:      owner.Name,
			},
			ownerObject,
		)
		if err != nil {
			fmt.Println(err)
			return false
		}
		return *ownerObject.Spec.Replicas != 0
	}
	return false
}

// err = hook.Client.Get(
// 	ctx,
// 	types.NamespacedName{
// 		Name: pod.Annotations["cache.spicedelver.me/cmtemplate"],
// 	},
// 	cmTemplate,
// )
