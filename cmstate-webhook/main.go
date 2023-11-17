package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	cachev1alpha1 "github.com/stollenaar/cm-injector-operator/api/v1alpha1"
	"k8s.io/api/admission/v1beta1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
	clientSet     *kubernetes.Clientset
)

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func init() {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	clientSet, err = kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
}

func handleMutate(w http.ResponseWriter, r *http.Request) {

	// read the body / request
	body, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
	}
	var review v1beta1.AdmissionReview
	if _, _, err := deserializer.Decode([]byte(body), nil, &review); err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
	}
	var pod *v1.Pod

	ar := review.Request

	if ar != nil {
		// get the Pod object and unmarshal it into its struct, if we cannot, we might as well stop here
		if err := json.Unmarshal(ar.Object.Raw, &pod); err != nil {
			fmt.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "%s", err)
			return
		}
	}
	fmt.Printf("Ready to handle %s event for %v\n", ar.Operation, *pod)
	if ar.Operation == v1beta1.Create {
		review.Response = handlePodCreate(pod)
	} else if ar.Operation == v1beta1.Delete {

	}

	resp, err := json.Marshal(review)
	if err != nil {
		fmt.Printf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}
	fmt.Printf("Ready to write reponse ...")
	if _, err := w.Write(resp); err != nil {
		fmt.Printf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

func main() {

	mux := http.NewServeMux()

	mux.HandleFunc("/", handleMutate)

	s := &http.Server{
		Addr:           ":8443",
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1048576
	}

	if _, err := os.Stat("/certs/tls.crt"); err == nil {
		fmt.Println("Running in https mode!")
		log.Fatal(s.ListenAndServeTLS("/certs/tls.crt", "/certs/tls.key"))
	} else {
		fmt.Println("Running in http mode!")
		log.Fatal(s.ListenAndServe())
	}
}

func handlePodCreate(pod *v1.Pod) *v1beta1.AdmissionResponse {
	response := &v1beta1.AdmissionResponse{
		Allowed: true,
	}

	if pod.Annotations["vault.hashicorp.com/agent-inject"] != "" &&
		pod.Annotations["vault.hashicorp.com/agent-internal-role"] != "" &&
		pod.Annotations["vault.hashicorp.com/agent-aws-role"] != "" {

		crdName := generateName(pod.Annotations)

		// get a list of our CRs
		cmState := &cachev1alpha1.CMState{}
		d, err := clientSet.RESTClient().
			Get().
			AbsPath("/apis/cache.spices.dev/v1alpha1").
			Namespace(pod.Namespace).
			Resource("CMState").
			Name(crdName).
			DoRaw(context.TODO())
		if err != nil && !apierrors.IsNotFound(err) {
			panic(err)
		} else if apierrors.IsNotFound(err) {
			// create the cmstate
			cmState = generateCMState(pod)

			body, _ := json.Marshal(cmState)

			_, err = clientSet.RESTClient().Post().
				AbsPath("/apis/cache.spices.dev/v1alpha1").
				Body(body).
				DoRaw(context.TODO())

			if err != nil {
				panic(err)
			}
		} else if err := json.Unmarshal(d, cmState); err != nil {
			panic(err)
		}

		patch := patchOperation{
			Op:   "add",
			Path: "/metadata/annotations",
			Value: map[string]string{
				"vault.hashicorp.com/agent-configmap": cmState.Name,
			},
		}
		pData, _ := json.Marshal(patch)
		response.Patch = pData
	}

	return response
}

// Generating a CMState used for later
func generateCMState(pod *v1.Pod) *cachev1alpha1.CMState {
	annotations := pod.GetAnnotations()

	return &cachev1alpha1.CMState{
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
	return strings.ReplaceAll(fmt.Sprintf("cmstate-%s-%s", annotations["vault.hashicorp.com/agent-internal-role"], annotations["vault.hashicorp.com/agent-aws-role"]), "_", "-")
}
