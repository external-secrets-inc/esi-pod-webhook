package admission

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/external-secrets-inc/esi-pod-webhook/pkg/annotations"
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
	return annotations.NeedsMutation(pod)
}

func (s *Server) hasEnvVarMode(pod *corev1.Pod) bool {
	return annotations.HasEnvVarMode(pod)
}

func (s *Server) needsImagePullSecrets(pod *corev1.Pod) bool {
	return annotations.NeedsImagePullSecrets(pod)
}

func (s *Server) hasFileMode(pod *corev1.Pod) bool {
	return annotations.HasFileMode(pod)
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

	// Create flag builder
	flagBuilder := annotations.NewFlagBuilder()

	// Get command and args from container
	var command []string
	if len(pod.Spec.Containers[0].Command) > 0 {
		command = pod.Spec.Containers[0].Command
	}

	var args []string
	if len(pod.Spec.Containers[0].Args) > 0 {
		args = pod.Spec.Containers[0].Args
	}

	flags, err := flagBuilder.BuildFlags(
		pod.Annotations,
		annotations.InitMode,
		annotations.WithOriginalCommand(command),
		annotations.WithOriginalArgs(args),
	)
	if err != nil {
		return fmt.Errorf("failed to build CLI flags: %v", err)
	}

	// Update container command to use esi-cli
	builder.Replace("/spec/containers/0/command",
		append([]string{"/secretless/bin/esi-cli"}, flags...))

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
