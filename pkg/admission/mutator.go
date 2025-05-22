package admission

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/external-secrets-inc/esi-pod-webhook/pkg/patch"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *Server) mutate(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	req := ar.Request

	// Parse pod
	pod := corev1.Pod{}
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		return s.responseWithError(fmt.Errorf("failed to decode pod: %v", err))
	}

	// Check if mutation is needed
	if !s.needsMutation(&pod) {
		return &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: true,
		}
	}

	// Get ExternalSecret name
	externalSecretName := pod.Annotations[AnnotationExternalSecret]
	if externalSecretName == "" {
		return s.responseWithError(fmt.Errorf("externalsecret annotation is required"))
	}

	// Create patch builder
	patchBuilder := patch.NewBuilder()
	if s.needsImagePullSecrets(&pod) {
		imagePullSecrets := strings.Split(pod.Annotations[AnnotationImagePullSecrets], ",")
		if err := s.handleImagePullSecrets(&pod, imagePullSecrets, patchBuilder); err != nil {
			return s.responseWithError(err)
		}
	}
	// Apply patches based on mode
	if s.hasEnvVarMode(&pod) {
		if err := s.handleEnvVarMode(&pod, externalSecretName, patchBuilder); err != nil {
			return s.responseWithError(err)
		}
	}

	if s.hasFileMode(&pod) {
		if err := s.handleFileMode(&pod, externalSecretName, patchBuilder); err != nil {
			return s.responseWithError(err)
		}
	}

	// Create patch
	patches := patchBuilder.Build()
	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return s.responseWithError(fmt.Errorf("failed to encode patches: %v", err))
	}

	// Create response
	pt := admissionv1.PatchTypeJSONPatch
	return &admissionv1.AdmissionResponse{
		UID:       req.UID,
		Allowed:   true,
		Patch:     patchBytes,
		PatchType: &pt,
	}
}

func (s *Server) needsMutation(pod *corev1.Pod) bool {
	if pod.Annotations[AnnotationSkip] == "true" {
		return false
	}
	return s.hasEnvVarMode(pod) || s.hasFileMode(pod)
}

func (s *Server) hasEnvVarMode(pod *corev1.Pod) bool {
	_, ok := pod.Annotations[AnnotationEnvVars]
	return ok
}

func (s *Server) needsImagePullSecrets(pod *corev1.Pod) bool {
	_, ok := pod.Annotations[AnnotationImagePullSecrets]
	return ok
}

func (s *Server) hasFileMode(pod *corev1.Pod) bool {
	_, ok := pod.Annotations[AnnotationFileSecrets]
	return ok
}

func (s *Server) handleImagePullSecrets(pod *corev1.Pod, pullSecrets []string, builder *patch.Builder) error {
	currentPullSecrets := pod.Spec.ImagePullSecrets
	for _, secret := range pullSecrets {
		currentPullSecrets = append(currentPullSecrets, corev1.LocalObjectReference{
			Name: secret,
		})
	}
	builder.Add("/spec/imagePullSecrets", currentPullSecrets)
	return nil
}

func (s *Server) handleEnvVarMode(pod *corev1.Pod, externalSecretName string, builder *patch.Builder) error {
	// Create init container
	container, err := s.initInjector.CreateContainer(pod, externalSecretName)
	if err != nil {
		return fmt.Errorf("failed to create init container: %v", err)
	}

	// Create volumes
	volumes, err := s.initInjector.CreateVolumes(pod)
	if err != nil {
		return fmt.Errorf("failed to create volumes: %v", err)
	}

	// Create volume mounts
	volumeMounts, err := s.initInjector.CreateVolumeMounts(pod)
	if err != nil {
		return fmt.Errorf("failed to create volume mounts: %v", err)
	}

	// Initialize initContainers array if it doesn't exist
	if len(pod.Spec.InitContainers) == 0 {
		builder.Add("/spec/initContainers", []corev1.Container{})
	}

	// Add init container
	builder.Add("/spec/initContainers/-", container)

	// Add volumes if any
	for _, volume := range volumes {
		builder.Add("/spec/volumes/-", volume)
	}

	// Add volume mounts to app container if any
	for _, mount := range volumeMounts {
		builder.Add("/spec/containers/0/volumeMounts/-", mount)
	}

	// Get original command from container
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

		// Update container command to use esi-cli
		builder.Replace(fmt.Sprintf("/spec/containers/%d/command", i), []string{
			"/secretless/bin/esi-cli",
			"--external-secrets=" + externalSecretName,
			"--binary-path=" + originalCommand[0],
			"--args=" + strings.Join(originalCommand[1:], ","),
			"--mode=init",
			"--inject-on-env=*", // Special value to get all keys
		})
	}

	return nil
}

func (s *Server) handleFileMode(pod *corev1.Pod, externalSecretName string, builder *patch.Builder) error {
	container, err := s.sidecarInjector.CreateContainer(pod, externalSecretName)
	if err != nil {
		return fmt.Errorf("failed to create sidecar container: %v", err)
	}

	volumes, err := s.sidecarInjector.CreateVolumes(pod)
	if err != nil {
		return fmt.Errorf("failed to create volumes: %v", err)
	}

	volumeMounts, err := s.sidecarInjector.CreateVolumeMounts(pod)
	if err != nil {
		return fmt.Errorf("failed to create volume mounts: %v", err)
	}

	// Add sidecar container
	builder.Add("/spec/containers/-", container)

	// Add volumes if any
	for _, volume := range volumes {
		builder.Add("/spec/volumes/-", volume)
	}

	// Add volume mounts to app container if any
	for _, mount := range volumeMounts {
		builder.Add("/spec/containers/0/volumeMounts/-", mount)
	}

	return nil
}

func (s *Server) responseWithError(err error) *admissionv1.AdmissionResponse {
	log.Printf("Error: %v", err)
	return &admissionv1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}
