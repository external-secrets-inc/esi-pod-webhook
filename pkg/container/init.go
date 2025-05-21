package container

import (
	corev1 "k8s.io/api/core/v1"
)

// InitContainerInjector handles init container injection
type InitContainerInjector struct {
	config *Config
}

// NewInitContainerInjector creates a new init container injector
func NewInitContainerInjector(config *Config) *InitContainerInjector {
	return &InitContainerInjector{
		config: config,
	}
}

// CreateContainer creates an init container for environment variable injection
func (i *InitContainerInjector) CreateContainer(pod *corev1.Pod, externalSecretName string) (*corev1.Container, error) {
	return &corev1.Container{
		Name:            "secretless-init",
		Image:           i.config.InitImage,
		ImagePullPolicy: i.config.ImagePullPolicy,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "secretless-bin",
				MountPath: "/secretless/bin",
			},
		},
	}, nil
}

// CreateVolumes creates volumes required for init container
func (i *InitContainerInjector) CreateVolumes(pod *corev1.Pod) ([]corev1.Volume, error) {
	return []corev1.Volume{
		{
			Name: "secretless-bin",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}, nil
}

// CreateVolumeMounts creates volume mounts required for init container
func (i *InitContainerInjector) CreateVolumeMounts(pod *corev1.Pod) ([]corev1.VolumeMount, error) {
	return []corev1.VolumeMount{
		{
			Name:      "secretless-bin",
			MountPath: "/secretless/bin",
		},
	}, nil
}
