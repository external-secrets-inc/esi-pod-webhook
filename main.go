package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	esv1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	secretlessBinary = "/bin/secretless-eso"
	sharedVolumeName = "secret-data"
	sharedVolumePath = "/mnt/secrets"
)

var (
	runtimeScheme = runtime.NewScheme()
)

func init() {
	// Add SecretStore type to the scheme
	log.Printf("Adding SecretStore v1 to scheme...")
	if err := esv1.AddToScheme(runtimeScheme); err != nil {
		log.Fatalf("Failed to add SecretStore v1 to scheme: %v", err)
	}
	log.Printf("Successfully added SecretStore v1 to scheme")
}

type webhookServer struct {
	server          *http.Server
	clientset       *kubernetes.Clientset
	dynamic         dynamic.Interface
	initImage       string
	sidecarImage    string
	imagePullPolicy corev1.PullPolicy
}

func newWebhookServer(kubeconfigPath, initImage, sidecarImage string, imagePullPolicy corev1.PullPolicy) (*webhookServer, error) {
	var config *rest.Config
	var err error

	if kubeconfigPath == "" {
		// Try in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get in-cluster config: %v", err)
		}
	} else {
		// Use kubeconfig file
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get config from kubeconfig: %v", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create dynamic client: %v", err)
	}

	return &webhookServer{
		clientset:       clientset,
		dynamic:         dynamicClient,
		initImage:       initImage,
		sidecarImage:    sidecarImage,
		imagePullPolicy: imagePullPolicy,
	}, nil
}

// Mutation webhook
func (ws *webhookServer) mutate(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	req := ar.Request
	log.Printf("Processing admission request for %s/%s", req.Namespace, req.Name)

	pod := corev1.Pod{}
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		log.Printf("Could not unmarshal raw object: %v", err)
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	log.Printf("Pod %s/%s annotations:", pod.Namespace, pod.Name)
	for k, v := range pod.Annotations {
		log.Printf("  %s: %s", k, v)
	}

	_, hasEnvVars := pod.Annotations["secretless.externalsecrets.com/env-vars"]
	_, hasFileSecrets := pod.Annotations["secretless.externalsecrets.com/file-secrets"]
	_, hasSkip := pod.Annotations["secretless.externalsecrets.com/skip"]
	_, hasExternalSecret := pod.Annotations["secretless.externalsecrets.com/externalsecret"]

	log.Printf("Pod %s/%s secretless annotations status:", pod.Namespace, pod.Name)
	log.Printf("  hasEnvVars: %v", hasEnvVars)
	log.Printf("  hasFileSecrets: %v", hasFileSecrets)
	log.Printf("  hasSkip: %v", hasSkip)
	log.Printf("  hasExternalSecret: %v", hasExternalSecret)

	if !hasEnvVars && !hasFileSecrets && !hasSkip && !hasExternalSecret {
		log.Printf("Pod %s/%s has no secretless annotations, allowing", pod.Namespace, pod.Name)
		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: true,
		}
	}

	if pod.Annotations["secretless.externalsecrets.com/skip"] == "true" {
		log.Printf("Skipping pod %s/%s due to skip annotation", pod.Namespace, pod.Name)
		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: true,
		}
	}

	if !needsMutation(&pod) {
		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: true,
		}
	}

	var patches []patchOperation

	// Get ExternalSecret name from annotation
	externalSecretName := pod.Annotations["secretless.externalsecrets.com/externalsecret"]
	if externalSecretName == "" {
		log.Printf("No ExternalSecret specified for pod %s", pod.Name)
		return &admissionv1.AdmissionResponse{
			UID: ar.Request.UID,
			Result: &metav1.Status{
				Message: "externalsecret annotation is required",
			},
		}
	}

	// Apply patches based on mode
	if hasEnvVarMode(&pod) {
		// For env vars, use init container to inject env vars
		patches = append(patches, ws.createEnvVarModePatches(&pod, externalSecretName)...)
	}

	if hasFileMode(&pod) {
		// For file mode, use sidecar container to inject files
		patches = append(patches, ws.createFileModePatches(&pod, externalSecretName)...)
	}

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	return &admissionv1.AdmissionResponse{
		UID:     ar.Request.UID,
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *admissionv1.PatchType {
			pt := admissionv1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}

func needsMutation(pod *corev1.Pod) bool {
	return hasEnvVarMode(pod) || hasFileMode(pod)
}

func hasEnvVarMode(pod *corev1.Pod) bool {
	_, ok := pod.Annotations["secretless.externalsecrets.com/env-vars"]
	return ok
}

func hasFileMode(pod *corev1.Pod) bool {
	_, ok := pod.Annotations["secretless.externalsecrets.com/file-secrets"]
	return ok
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func (ws *webhookServer) createEnvVarModePatches(pod *corev1.Pod, externalSecretName string) []patchOperation {
	var patches []patchOperation

	// Create shared volume for secretless binary
	binaryVolume := corev1.Volume{
		Name: "secretless-bin",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	// Add volumes
	patches = append(patches, patchOperation{
		Op:    "add",
		Path:  "/spec/volumes/-",
		Value: binaryVolume,
	})

	// Create init container
	initContainer := corev1.Container{
		Name:            "secretless-init",
		Image:           ws.initImage,
		ImagePullPolicy: ws.imagePullPolicy,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "secretless-bin",
				MountPath: "/secretless/bin",
			},
		},
	}

	// Add init container
	patches = append(patches, patchOperation{
		Op:    "add",
		Path:  "/spec/initContainers",
		Value: []corev1.Container{initContainer},
	})

	// Modify container command to use secretless-eso with SecretStore
	for i := range pod.Spec.Containers {
		var originalCommand []string
		if len(pod.Spec.Containers[i].Command) > 0 {
			originalCommand = pod.Spec.Containers[i].Command
		} else if len(pod.Spec.Containers[i].Args) > 0 {
			originalCommand = pod.Spec.Containers[i].Args
		}

		// If no command or args specified, use the default shell
		if len(originalCommand) == 0 {
			originalCommand = []string{"/bin/sh", "-c", "while true; do env | grep API; sleep 10; done"}
		}

		pod.Spec.Containers[i].Command = []string{
			"/secretless/bin/esi-cli",
			"--external-secrets=" + externalSecretName,
			"--binary-path=" + originalCommand[0],
			"--args=" + strings.Join(originalCommand[1:], ","),
			"--mode=init",
			"--inject-on-env=*", // Special value to get all keys
		}

		// Add volume mounts
		pod.Spec.Containers[i].VolumeMounts = append(
			pod.Spec.Containers[i].VolumeMounts,
			corev1.VolumeMount{
				Name:      "secretless-bin",
				MountPath: "/secretless/bin",
				ReadOnly:  true,
			},
		)

		// Create patch for volume mounts
		patches = append(patches, patchOperation{
			Op:    "replace",
			Path:  fmt.Sprintf("/spec/containers/%d/volumeMounts", i),
			Value: pod.Spec.Containers[i].VolumeMounts,
		})

		// Create patch for command
		patches = append(patches, patchOperation{
			Op:    "replace",
			Path:  fmt.Sprintf("/spec/containers/%d/command", i),
			Value: pod.Spec.Containers[i].Command,
		})
	}

	return patches
}

func (ws *webhookServer) createFileModePatches(pod *corev1.Pod, externalSecretName string) []patchOperation {
	var patches []patchOperation

	// Create emptyDir volume for secrets
	secretsVolume := corev1.Volume{
		Name: "secrets",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium: corev1.StorageMediumMemory,
			},
		},
	}

	// Add volumes
	patches = append(patches, patchOperation{
		Op:    "add",
		Path:  "/spec/volumes/-",
		Value: secretsVolume,
	})

	// Add volume mount to app container
	appVolumeMount := corev1.VolumeMount{
		Name:      "secrets",
		MountPath: "/secrets",
	}
	patches = append(patches, patchOperation{
		Op:    "add",
		Path:  "/spec/containers/0/volumeMounts/-",
		Value: appVolumeMount,
	})

	// Create sidecar container
	sidecarContainer := corev1.Container{
		Name:            "secretless-sidecar",
		Image:           ws.sidecarImage,
		ImagePullPolicy: ws.imagePullPolicy,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "secrets",
				MountPath: "/secrets",
			},
		},
		Args: []string{
			"--external-secrets=" + externalSecretName,
			"--mode=daemon",
			"--inject-on-file=/secrets/config.json=" + externalSecretName,
		},
	}

	// Add sidecar container
	patches = append(patches, patchOperation{
		Op:    "add",
		Path:  "/spec/containers/-",
		Value: sidecarContainer,
	})

	return patches
}

func (ws *webhookServer) serve(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := io.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	// Verify content type
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		log.Printf("Content-Type=%s, expect application/json", contentType)
		http.Error(w, "invalid Content-Type, expect `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	// Parse admission review request
	admissionReview := admissionv1.AdmissionReview{}
	if err := json.Unmarshal(body, &admissionReview); err != nil {
		log.Printf("Can't decode body: %v", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Process the admission request
	admissionResponse := ws.mutate(&admissionReview)
	admissionReview.Response = admissionResponse

	// Send response
	resp, err := json.Marshal(admissionReview)
	if err != nil {
		log.Printf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(resp); err != nil {
		log.Printf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

func main() {
	var tlsKey string
	var tlsCert string
	var port int
	var kubeconfigPath string
	var initImage string
	var sidecarImage string
	var imagePullPolicy string

	flag.StringVar(&tlsKey, "tls-key", "", "Path to the TLS key")
	flag.StringVar(&tlsCert, "tls-cert", "", "Path to the TLS certificate")
	flag.IntVar(&port, "port", 8443, "Webhook server port")
	flag.StringVar(&kubeconfigPath, "kube-config", "", "Paths to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&initImage, "init-image", "esi-cli-init:test", "Image to use for init container")
	flag.StringVar(&sidecarImage, "sidecar-image", "esi-cli-sidecar:test", "Image to use for sidecar container")
	flag.StringVar(&imagePullPolicy, "image-pull-policy", string(corev1.PullIfNotPresent), "Image pull policy for injected containers (Always, Never, IfNotPresent)")
	flag.Parse()

	// Create shared mux for both servers
	mux := http.NewServeMux()

	// Create webhook server with Kubernetes client
	whsvr, err := newWebhookServer(kubeconfigPath, initImage, sidecarImage, corev1.PullPolicy(imagePullPolicy))
	if err != nil {
		log.Fatalf("Failed to create webhook server: %v", err)
	}

	// Add webhook endpoints
	mux.HandleFunc("/mutate", whsvr.serve)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte("ok"))
		if err != nil {
			log.Printf("Error writing health response: %v", err)
		}
	})

	// Load TLS cert/key
	pair, err := tls.LoadX509KeyPair(tlsCert, tlsKey)
	if err != nil {
		log.Fatalf("Failed to load key pair: %v", err)
	}

	// Set up HTTPS server
	whsvr.server = &http.Server{
		Addr:      fmt.Sprintf(":%v", port),
		Handler:   mux,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
	}

	// Start webhook server in new routine
	go func() {
		log.Printf("Starting webhook server on port %d", port)
		if err := whsvr.server.ListenAndServeTLS("", ""); err != nil {
			log.Fatalf("Failed to listen and serve webhook server: %v", err)
		}
	}()

	log.Printf("Server started")

	// Listening OS shutdown signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	log.Printf("Got OS shutdown signal, shutting down webhook server gracefully...")
	err = whsvr.server.Shutdown(context.Background())
	if err != nil {
		log.Printf("Error shutting down server: %v", err)
	} else {
		log.Printf("Webhook server shut down gracefully")
	}
}
