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
	"time"

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

	// Create ConfigMap without owner reference (will be added by init container)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-secretless-config", pod.Name),
			Namespace: pod.Namespace,
		},
		Data: config,
	}

	// Check if ConfigMap exists
	existing, err := ws.clientset.CoreV1().ConfigMaps(pod.Namespace).Get(context.Background(), configMap.Name, metav1.GetOptions{})
	if err == nil {
		// ConfigMap exists, update it
		log.Printf("ConfigMap %s/%s exists, updating...", pod.Namespace, configMap.Name)
		configMap.ResourceVersion = existing.ResourceVersion
		_, err = ws.clientset.CoreV1().ConfigMaps(pod.Namespace).Update(context.Background(), configMap, metav1.UpdateOptions{})
	} else if k8serrors.IsNotFound(err) {
		// ConfigMap doesn't exist, create it
		log.Printf("ConfigMap %s/%s doesn't exist, creating...", pod.Namespace, configMap.Name)
		_, err = ws.clientset.CoreV1().ConfigMaps(pod.Namespace).Create(context.Background(), configMap, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create secretless ConfigMap: %v", err)
		}
		// wait for ConfigMap to be created
		for {
			_, err = ws.clientset.CoreV1().ConfigMaps(pod.Namespace).Get(context.Background(), configMap.Name, metav1.GetOptions{})
			if err == nil {
				break
			}
			if !k8serrors.IsNotFound(err) {
				return fmt.Errorf("failed to get secretless ConfigMap: %v", err)
			}
			time.Sleep(1 * time.Second)
		}
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

	// Create ConfigMap without owner reference (will be added by init container)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-secretstore-and-externalsecret", pod.Name),
			Namespace: pod.Namespace,
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
		log.Printf("ConfigMap %s/%s exists, updating...", pod.Namespace, configMap.Name)
		configMap.ResourceVersion = existing.ResourceVersion
		_, err = ws.clientset.CoreV1().ConfigMaps(pod.Namespace).Update(context.Background(), configMap, metav1.UpdateOptions{})
	} else if k8serrors.IsNotFound(err) {
		// ConfigMap doesn't exist, create it
		log.Printf("ConfigMap %s/%s doesn't exist, creating...", pod.Namespace, configMap.Name)
		_, err = ws.clientset.CoreV1().ConfigMaps(pod.Namespace).Create(context.Background(), configMap, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create ConfigMap: %v", err)
		}
		// wait for ConfigMap to be created
		for {
			_, err = ws.clientset.CoreV1().ConfigMaps(pod.Namespace).Get(context.Background(), configMap.Name, metav1.GetOptions{})
			if err == nil {
				break
			}
			if !k8serrors.IsNotFound(err) {
				return fmt.Errorf("failed to get ConfigMap: %v", err)
			}
			time.Sleep(1 * time.Second)
		}
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
			UID: ar.Request.UID,
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
				UID: ar.Request.UID,
				Result: &metav1.Status{
					Message: fmt.Sprintf("ExternalSecret %s not found in namespace %s", externalSecretName, pod.Namespace),
				},
			}
		}
		log.Printf("Error getting ExternalSecret: %v", err)
		return &admissionv1.AdmissionResponse{
			UID: ar.Request.UID,
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(externalSecretObj.UnstructuredContent(), externalSecret); err != nil {
		log.Printf("Error converting ExternalSecret: %v", err)
		return &admissionv1.AdmissionResponse{
			UID: ar.Request.UID,
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
				UID: ar.Request.UID,
				Result: &metav1.Status{
					Message: fmt.Sprintf("SecretStore %s not found in namespace %s", externalSecret.Spec.SecretStoreRef.Name, pod.Namespace),
				},
			}
		}
		log.Printf("Error getting SecretStore: %v", err)
		return &admissionv1.AdmissionResponse{
			UID: ar.Request.UID,
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(secretStoreObj.UnstructuredContent(), secretStore); err != nil {
		log.Printf("Error converting SecretStore: %v", err)
		return &admissionv1.AdmissionResponse{
			UID: ar.Request.UID,
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	log.Printf("Got SecretStore response: %+v", secretStore)

	// Create ConfigMaps
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

	// Create shared volume for secretless binary
	binaryVolume := corev1.Volume{
		Name: "secretless-bin",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	// Create volume for SecretStore and ExternalSecret
	configVolume := corev1.Volume{
		Name: fmt.Sprintf("%s-secretstore-and-externalsecret", pod.Name),
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf("%s-secretstore-and-externalsecret", pod.Name),
				},
			},
		},
	}

	// Add volumes
	patches = append(patches,
		patchOperation{
			Op:    "add",
			Path:  "/spec/volumes/-",
			Value: binaryVolume,
		},
		patchOperation{
			Op:    "add",
			Path:  "/spec/volumes/-",
			Value: configVolume,
		},
	)

	// Create init container
	initContainer := corev1.Container{
		Name:  "secretless-init",
		Image: "secretless-eso-init:latest",
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
		originalCommand := pod.Spec.Containers[i].Command
		pod.Spec.Containers[i].Command = []string{
			"/secretless/bin/secretless-eso",
			"--mode=envVarInjection",
			"--binary=" + strings.Join(originalCommand, " "),
		}
		
		// Add volume mounts
		pod.Spec.Containers[i].VolumeMounts = append(
			pod.Spec.Containers[i].VolumeMounts,
			corev1.VolumeMount{
				Name:      "secretless-bin",
				MountPath: "/secretless/bin",
				ReadOnly:  true,
			},
			corev1.VolumeMount{
				Name:      fmt.Sprintf("%s-secretstore-and-externalsecret", pod.Name),
				MountPath: "/etc/secretstore-and-externalsecret",
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

func createFileModePatches(pod *corev1.Pod) []patchOperation {
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

	// Create volume for SecretStore and ExternalSecret
	configVolume := corev1.Volume{
		Name: fmt.Sprintf("%s-secretstore-and-externalsecret", pod.Name),
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf("%s-secretstore-and-externalsecret", pod.Name),
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

	// Add volumes
	patches = append(patches,
		patchOperation{
			Op:    "add",
			Path:  "/spec/volumes/-",
			Value: secretsVolume,
		},
		patchOperation{
			Op:    "add",
			Path:  "/spec/volumes/-",
			Value: configVolume,
		},
		patchOperation{
			Op:    "add",
			Path:  "/spec/volumes/-",
			Value: secretlessConfigVolume,
		},
	)

	// Create sidecar container
	sidecarContainer := corev1.Container{
		Name:  "secretless-sidecar",
		Image: "secretless-eso-sidecar:latest",
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "secrets",
				MountPath: "/secrets",
			},
			{
				Name:      fmt.Sprintf("%s-secretstore-and-externalsecret", pod.Name),
				MountPath: "/etc/secretstore-and-externalsecret",
				ReadOnly:  true,
			},
			{
				Name:      "secretless-config",
				MountPath: "/etc/secretless",
				ReadOnly:  true,
			},
		},
		Args: []string{
			"--mode=fileInjection",
			"--config=/etc/secretless/config.json",
		},
	}

	// Add sidecar container
	patches = append(patches, patchOperation{
		Op:   "add",
		Path: "/spec/containers/-",
		Value: sidecarContainer,
	})

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

	flag.StringVar(&tlsKey, "tls-key", "", "Path to the TLS key")
	flag.StringVar(&tlsCert, "tls-cert", "", "Path to the TLS certificate")
	flag.IntVar(&port, "port", 8443, "Webhook server port")
	flag.StringVar(&kubeconfigPath, "kube-config", "", "Paths to a kubeconfig. Only required if out-of-cluster.")
	flag.Parse()

	// Create shared mux for both servers
	mux := http.NewServeMux()


	// Create webhook server with Kubernetes client
	whsvr, err := newWebhookServer(kubeconfigPath)
	if err != nil {
		log.Fatalf("Failed to create webhook server: %v", err)
	}

	// Add webhook endpoints
	mux.HandleFunc("/mutate", whsvr.serve)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
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
	whsvr.server.Shutdown(context.Background())
}
