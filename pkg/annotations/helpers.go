package annotations

import (
	corev1 "k8s.io/api/core/v1"
)

// HasEnvVarMode checks if the pod has environment variable mode enabled
func HasEnvVarMode(pod *corev1.Pod) bool {
	_, ok := pod.Annotations[AnnotationEnvVars]
	return ok
}

// HasFileMode checks if the pod has file mode enabled
func HasFileMode(pod *corev1.Pod) bool {
	_, ok := pod.Annotations[AnnotationFileSecrets]
	return ok
}

// NeedsMutation checks if the pod needs mutation
func NeedsMutation(pod *corev1.Pod) bool {
	if pod.Annotations[AnnotationSkip] == "true" {
		return false
	}
	return HasEnvVarMode(pod) || HasFileMode(pod)
}

// NeedsImagePullSecrets checks if the pod needs image pull secrets
func NeedsImagePullSecrets(pod *corev1.Pod) bool {
	_, ok := pod.Annotations[AnnotationImagePullSecrets]
	return ok
}
