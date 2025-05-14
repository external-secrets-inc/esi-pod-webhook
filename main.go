package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"

	esv1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1"
)

const (
	secretlessBinary  = "secretless-eso"
	sharedVolumeName  = "secret-data"
	sharedVolumePath  = "/mnt/secrets"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs       = serializer.NewCodecFactory(runtimeScheme)
	deserializer = codecs.UniversalDeserializer()
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
	server *http.Server
	clientset *kubernetes.Clientset
	dynamic   dynamic.Interface
}

func newWebhookServer(kubeconfigPath string) (*webhookServer, error) {
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
		clientset: clientset,
		dynamic:   dynamicClient,
	}, nil
}

func (ws *webhookServer) createSecretlessConfigMap(pod *corev1.Pod, uid types.UID) error {
	// Parse file-secrets annotation
	fileSecrets := pod.Annotations["secretless.externalsecrets.com/file-secrets"]
	if fileSecrets == "" {
		return nil
	}

	// Create config
	config := map[string]string{
		"config.json": fmt.Sprintf(`{
			"external_secrets": [
				{
					"name": "%s",
					"mount_path": "%s"
				}
			]
		}`, fileSecrets, pod.Name),
	}

	// Create ConfigMap
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-secretless-config", pod.Name),
			Namespace: pod.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Pod",
					Name:       pod.Name,
					UID:        uid,
				},
			},
		},
		Data: config,
	}

	// Check if ConfigMap exists
	existing, err := ws.clientset.CoreV1().ConfigMaps(pod.Namespace).Get(context.Background(), configMap.Name, metav1.GetOptions{})
	if err == nil {
		// ConfigMap exists, update it
		configMap.ResourceVersion = existing.ResourceVersion
		_, err = ws.clientset.CoreV1().ConfigMaps(pod.Namespace).Update(context.Background(), configMap, metav1.UpdateOptions{})
	} else if k8serrors.IsNotFound(err) {
		// ConfigMap doesn't exist, create it
		_, err = ws.clientset.CoreV1().ConfigMaps(pod.Namespace).Create(context.Background(), configMap, metav1.CreateOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to create or update secretless ConfigMap: %v", err)
	}

	return nil
}

func (ws *webhookServer) createSecretStoreAndExternalSecretConfigMap(pod *corev1.Pod, externalSecret *esv1.ExternalSecret, secretStore *esv1.SecretStore, uid types.UID) error {
	// Convert ExternalSecret and SecretStore to YAML
	externalSecretYAML, err := yaml.Marshal(externalSecret)
	if err != nil {
		return fmt.Errorf("failed to marshal ExternalSecret: %v", err)
	}

	secretStoreYAML, err := yaml.Marshal(secretStore)
	if err != nil {
		return fmt.Errorf("failed to marshal SecretStore: %v", err)
	}

	// Create ConfigMap
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-secretstore-and-externalsecret", pod.Name),
			Namespace: pod.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Pod",
					Name:       pod.Name,
					UID:        uid,
				},
			},
		},
		Data: map[string]string{
			"externalsecret.yaml": string(externalSecretYAML),
			"secretstore.yaml":    string(secretStoreYAML),
		},
	}

	// Check if ConfigMap exists
	existing, err := ws.clientset.CoreV1().ConfigMaps(pod.Namespace).Get(context.Background(), configMap.Name, metav1.GetOptions{})
	if err == nil {
		// ConfigMap exists, update it
		configMap.ResourceVersion = existing.ResourceVersion
		_, err = ws.clientset.CoreV1().ConfigMaps(pod.Namespace).Update(context.Background(), configMap, metav1.UpdateOptions{})
	} else if k8serrors.IsNotFound(err) {
		// ConfigMap doesn't exist, create it
		_, err = ws.clientset.CoreV1().ConfigMaps(pod.Namespace).Create(context.Background(), configMap, metav1.CreateOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to create ConfigMap: %v", err)
	}

	return nil
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
			UID: ar.Request.UID,
			Allowed: true,
		}
	}

	if pod.Annotations["secretless.externalsecrets.com/skip"] == "true" {
		log.Printf("Skipping pod %s/%s due to skip annotation", pod.Namespace, pod.Name)
		return &admissionv1.AdmissionResponse{
			UID: ar.Request.UID,
			Allowed: true,
		}
	}

	if !needsMutation(&pod) {
		return &admissionv1.AdmissionResponse{
			UID: ar.Request.UID,
			Allowed: true,
		}
	}

	// Get ExternalSecret name from annotation
	externalSecretName := pod.Annotations["secretless.externalsecrets.com/externalsecret"]
	if externalSecretName == "" {
		log.Printf("No ExternalSecret specified for pod %s", pod.Name)
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: "externalsecret annotation is required",
			},
		}
	}

	log.Printf("Getting ExternalSecret %s in namespace %s...", externalSecretName, pod.Namespace)

	// Get ExternalSecret
	externalSecret := &esv1.ExternalSecret{}
	externalSecretGVR := schema.GroupVersionResource{
		Group:    "external-secrets.io",
		Version:  "v1",
		Resource: "externalsecrets",
	}

	externalSecretObj, err := ws.dynamic.Resource(externalSecretGVR).Namespace(pod.Namespace).Get(context.TODO(), externalSecretName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Printf("ExternalSecret %s not found in namespace %s", externalSecretName, pod.Namespace)
			return &admissionv1.AdmissionResponse{
				Result: &metav1.Status{
					Message: fmt.Sprintf("ExternalSecret %s not found in namespace %s", externalSecretName, pod.Namespace),
				},
			}
		}
		log.Printf("Error getting ExternalSecret: %v", err)
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(externalSecretObj.UnstructuredContent(), externalSecret); err != nil {
		log.Printf("Error converting ExternalSecret: %v", err)
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	log.Printf("Got ExternalSecret response: %+v", externalSecret)

	// Get referenced SecretStore
	secretStore := &esv1.SecretStore{}
	secretStoreGVR := schema.GroupVersionResource{
		Group:    "external-secrets.io",
		Version:  "v1",
		Resource: "secretstores",
	}

	secretStoreObj, err := ws.dynamic.Resource(secretStoreGVR).Namespace(pod.Namespace).Get(context.TODO(), externalSecret.Spec.SecretStoreRef.Name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Printf("SecretStore %s not found in namespace %s", externalSecret.Spec.SecretStoreRef.Name, pod.Namespace)
			return &admissionv1.AdmissionResponse{
				Result: &metav1.Status{
					Message: fmt.Sprintf("SecretStore %s not found in namespace %s", externalSecret.Spec.SecretStoreRef.Name, pod.Namespace),
				},
			}
		}
		log.Printf("Error getting SecretStore: %v", err)
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(secretStoreObj.UnstructuredContent(), secretStore); err != nil {
		log.Printf("Error converting SecretStore: %v", err)
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	log.Printf("Got SecretStore response: %+v", secretStore)

	if err := ws.createSecretStoreAndExternalSecretConfigMap(&pod, externalSecret, secretStore, req.UID); err != nil {
		return &admissionv1.AdmissionResponse{
			UID: ar.Request.UID,
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	if err := ws.createSecretlessConfigMap(&pod, req.UID); err != nil {
		return &admissionv1.AdmissionResponse{
			UID: ar.Request.UID,
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	// Convert unstructured SecretStore to typed SecretStore
	typedSecretStore := &esv1.SecretStore{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(secretStoreObj.UnstructuredContent(), typedSecretStore); err != nil {
		return &admissionv1.AdmissionResponse{
			UID: ar.Request.UID,
			Result: &metav1.Status{
				Message: fmt.Sprintf("failed to convert unstructured SecretStore: %v", err),
			},
		}
	}

	// Convert unstructured ExternalSecret to typed ExternalSecret
	externalSecretTyped := &esv1.ExternalSecret{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(externalSecretObj.UnstructuredContent(), externalSecretTyped); err != nil {
		return &admissionv1.AdmissionResponse{
			UID: ar.Request.UID,
			Result: &metav1.Status{
				Message: fmt.Sprintf("failed to convert unstructured ExternalSecret: %v", err),
			},
		}
	}

	if err := ws.createSecretStoreAndExternalSecretConfigMap(&pod, externalSecretTyped, typedSecretStore, req.UID); err != nil {
		return &admissionv1.AdmissionResponse{
			UID: ar.Request.UID,
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	if err := ws.createSecretlessConfigMap(&pod, req.UID); err != nil {
		return &admissionv1.AdmissionResponse{
			UID: ar.Request.UID,
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	var patches []patchOperation

	if hasEnvVarMode(&pod) {
		patches = append(patches, createEnvVarModePatches(&pod)...)
	}

	if hasFileMode(&pod) {
		patches = append(patches, createFileModePatches(&pod)...)
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
		UID: ar.Request.UID,
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

func createEnvVarModePatches(pod *corev1.Pod) []patchOperation {
	var patches []patchOperation

	// Get SecretStore name from annotation
	storeName := pod.Annotations["secretless.externalsecrets.com/secretstore"]
	if storeName == "" {
		log.Printf("No SecretStore specified for pod %s, using 'default'", pod.Name)
		storeName = "default"
	}

	// Create ConfigMap for SecretStore
	configVolume := corev1.Volume{
		Name: "secretstore-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf("%s-secretstore", pod.Name),
				},
			},
		},
	}

	patches = append(patches, patchOperation{
		Op:    "add",
		Path:  "/spec/volumes/-",
		Value: configVolume,
	})

	// Modify container command to use secretless-eso with SecretStore
	for i := range pod.Spec.Containers {
		originalCommand := pod.Spec.Containers[i].Command
		pod.Spec.Containers[i].Command = []string{
			secretlessBinary,
			"--mode=envVarInjection",
			"--binary=" + strings.Join(originalCommand, " "),
		}
		
		// Add volume mount for SecretStore config
		pod.Spec.Containers[i].VolumeMounts = append(
			pod.Spec.Containers[i].VolumeMounts,
			corev1.VolumeMount{
				Name:      "secretstore-config",
				MountPath: "/etc/secretstore",
				ReadOnly:  true,
			},
		)
	}

	return patches
}

func createFileModePatches(pod *corev1.Pod) []patchOperation {
	var patches []patchOperation

	// Get SecretStore name from annotation
	storeName := pod.Annotations["secretless.externalsecrets.com/secretstore"]
	if storeName == "" {
		log.Printf("No SecretStore specified for pod %s, using 'default'", pod.Name)
		storeName = "default"
	}

	// Create shared volume for secrets
	secretVolume := corev1.Volume{
		Name: sharedVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium: corev1.StorageMediumMemory,
			},
		},
	}

	// Create ConfigMap for SecretStore
	configVolume := corev1.Volume{
		Name: "secretstore-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf("%s-secretstore", pod.Name),
				},
			},
		},
	}

	// Create ConfigMap for secretless config
	secretlessConfigVolume := corev1.Volume{
		Name: "secretless-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf("%s-secretless-config", pod.Name),
				},
			},
		},
	}

	// Create sidecar container
	sidecar := corev1.Container{
		Name:  "secretless-sidecar",
		Image: "secretless-eso:latest",
		Command: []string{
			secretlessBinary,
			"--mode=fileInjection",
			"--config=/etc/secretless/config.json",
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      sharedVolumeName,
				MountPath: sharedVolumePath,
			},
			{
				Name:      "secretless-config",
				MountPath: "/etc/secretless",
			},
			{
				Name:      "secretstore-config",
				MountPath: "/etc/secretstore",
				ReadOnly:  true,
			},
		},
	}

	// Only add secretstore-config volume if env var mode is not enabled
	_, hasEnvVars := pod.Annotations["secretless.externalsecrets.com/env-vars"]

	patches = append(patches,
		patchOperation{
			Op:    "add",
			Path:  "/spec/volumes/-",
			Value: secretVolume,
		},
	)

	// Only add secretstore-config if env var mode is not enabled
	if !hasEnvVars {
		patches = append(patches,
			patchOperation{
				Op:    "add",
				Path:  "/spec/volumes/-",
				Value: configVolume,
			},
		)
	}

	patches = append(patches,
		patchOperation{
			Op:    "add",
			Path:  "/spec/volumes/-",
			Value: secretlessConfigVolume,
		},
		patchOperation{
			Op:    "add",
			Path:  "/spec/containers/-",
			Value: sidecar,
		},
	)

	// Add volume mounts to all containers
	for i := range pod.Spec.Containers {
		pod.Spec.Containers[i].VolumeMounts = append(
			pod.Spec.Containers[i].VolumeMounts,
			corev1.VolumeMount{
				Name:      sharedVolumeName,
				MountPath: sharedVolumePath,
			},
		)
	}

	return patches
}

func (ws *webhookServer) serve(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
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
	var debugPort int

	flag.StringVar(&tlsKey, "tls-key", "", "Path to the TLS key")
	flag.StringVar(&tlsCert, "tls-cert", "", "Path to the TLS certificate")
	flag.IntVar(&port, "port", 8443, "Webhook server port")
	flag.StringVar(&kubeconfigPath, "kube-config", "", "Paths to a kubeconfig. Only required if out-of-cluster.")
	flag.IntVar(&debugPort, "debug-port", 40000, "Debug server port")
	flag.Parse()

	// Start debug server if debug port is specified
	if debugPort > 0 {
		go func() {
			log.Printf("Starting debug server on port %d", debugPort)
			mux := http.NewServeMux()
			mux.HandleFunc("/debug/info", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"status": "running",
					"port": port,
					"debug_port": debugPort,
				})
			})
			if err := http.ListenAndServe(fmt.Sprintf(":%d", debugPort), mux); err != nil {
				log.Printf("Debug server failed: %v", err)
			}
		}()
	}

	pair, err := tls.LoadX509KeyPair(tlsCert, tlsKey)
	if err != nil {
		log.Fatalf("Failed to load key pair: %v", err)
	}

	// Create webhook server with Kubernetes client
	whsvr, err := newWebhookServer(kubeconfigPath)
	if err != nil {
		log.Fatalf("Failed to create webhook server: %v", err)
	}

	// Set up HTTP server
	whsvr.server = &http.Server{
		Addr:      fmt.Sprintf(":%v", port),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
	}

	// Define HTTP server and server handler
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", whsvr.serve)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	whsvr.server.Handler = mux

	// Start webhook server in new routine
	go func() {
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
	whsvr.server.Shutdown(context.Background())
}
