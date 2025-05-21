package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/external-secrets-inc/esi-pod-webhook/pkg/admission"
	"github.com/external-secrets-inc/esi-pod-webhook/pkg/container"
	"github.com/external-secrets-inc/esi-pod-webhook/pkg/k8s"
	corev1 "k8s.io/api/core/v1"
)

func main() {
	var (
		port            = flag.Int("port", 8443, "Webhook server port")
		tlsCertPath     = flag.String("tls-cert", "", "Path to TLS certificate file")
		tlsKeyPath      = flag.String("tls-key", "", "Path to TLS key file")
		kubeconfigPath  = flag.String("kube-config", "", "Path to kubeconfig file")
		initImage       = flag.String("init-image", "", "Init container image")
		sidecarImage    = flag.String("sidecar-image", "", "Sidecar container image")
		imagePullPolicy = flag.String("image-pull-policy", string(corev1.PullIfNotPresent), "Image pull policy")
	)
	flag.Parse()

	// Create Kubernetes client
	k8sClient, err := k8s.NewClient(*kubeconfigPath)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// Create webhook configuration
	containerConfig := &container.Config{
		InitImage:       *initImage,
		SidecarImage:    *sidecarImage,
		ImagePullPolicy: corev1.PullPolicy(*imagePullPolicy),
	}

	webhookConfig := &admission.Config{
		Port:         *port,
		TLSCertPath:  *tlsCertPath,
		TLSKeyPath:   *tlsKeyPath,
		ContainerCfg: containerConfig,
	}

	// Create and start webhook server
	server, err := admission.NewServer(webhookConfig, k8sClient)
	if err != nil {
		log.Fatalf("Failed to create webhook server: %v", err)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in background
	go func() {
		log.Printf("Starting webhook server on port %d", *port)
		if err := server.Start(ctx); err != nil {
			log.Printf("Server error: %v", err)
			cancel()
		}
	}()

	// Wait for shutdown signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	log.Printf("Received shutdown signal, gracefully shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}
