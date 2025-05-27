package container

import (
	"fmt"

	"github.com/external-secrets-inc/esi-pod-webhook/pkg/annotations"
	corev1 "k8s.io/api/core/v1"
)

// SidecarContainerInjector handles sidecar container injection
type SidecarContainerInjector struct {
	config *Config
}

// NewSidecarContainerInjector creates a new sidecar container injector
func NewSidecarContainerInjector(config *Config) *SidecarContainerInjector {
	return &SidecarContainerInjector{
		config: config,
	}
}

// CreateContainer creates a sidecar container for file injection
func (s *SidecarContainerInjector) CreateContainer(pod *corev1.Pod, externalSecretName string) (*corev1.Container, error) {
	volumeMounts, err := s.CreateVolumeMounts(pod)
	if err != nil {
		return nil, err
	}

	// Create flag builder
	flagBuilder := annotations.NewFlagBuilder()

	// Build CLI flags
	flags, err := flagBuilder.BuildFlags(
		pod.Annotations,
		annotations.DaemonMode,
		annotations.WithFilePath("/secrets/secrets.json"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build CLI flags: %v", err)
	}

	return &corev1.Container{
		Name:            "secretless-sidecar",
		Image:           s.config.SidecarImage,
		ImagePullPolicy: s.config.ImagePullPolicy,
		VolumeMounts:    volumeMounts,
		Args:           flags,
	}, nil
}

// CreateVolumes creates volumes required for sidecar container
func (s *SidecarContainerInjector) CreateVolumes(pod *corev1.Pod) ([]corev1.Volume, error) {
	return []corev1.Volume{
		{
			Name: "secrets",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium: corev1.StorageMediumMemory,
				},
			},
		},
	}, nil
}

// CreateVolumeMounts creates volume mounts required for sidecar container
func (s *SidecarContainerInjector) CreateVolumeMounts(pod *corev1.Pod) ([]corev1.VolumeMount, error) {
	return []corev1.VolumeMount{
		{
			Name:      "secrets",
			MountPath: "/secrets",
		},
	}, nil
}
