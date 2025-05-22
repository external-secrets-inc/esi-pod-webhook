package container

import (
	corev1 "k8s.io/api/core/v1"
)

// Config holds configuration for container injection
type Config struct {
	InitImage       string
	SidecarImage    string
	ImagePullPolicy corev1.PullPolicy
}

// Injector defines the interface for container injection
type Injector interface {
	CreateContainer(pod *corev1.Pod, externalSecretName string) (*corev1.Container, error)
	CreateVolumes(pod *corev1.Pod) ([]corev1.Volume, error)
	CreateVolumeMounts(pod *corev1.Pod) ([]corev1.VolumeMount, error)
}
