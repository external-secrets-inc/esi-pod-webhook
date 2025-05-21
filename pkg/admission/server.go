package admission

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/external-secrets-inc/esi-pod-webhook/pkg/container"
	"github.com/external-secrets-inc/esi-pod-webhook/pkg/k8s"
	admissionv1 "k8s.io/api/admission/v1"
)

// NewServer creates a new webhook server
func NewServer(config *Config, k8sClient *k8s.Client) (*Server, error) {
	cert, err := tls.LoadX509KeyPair(config.TLSCertPath, config.TLSKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load key pair: %v", err)
	}

	server := &Server{
		k8sClient: k8sClient,
		tlsConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
		initInjector:    container.NewInitContainerInjector(config.ContainerCfg),
		sidecarInjector: container.NewSidecarContainerInjector(config.ContainerCfg),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", server.handleMutate)
	mux.HandleFunc("/health", server.handleHealth)

	server.httpServer = &http.Server{
		Addr:      fmt.Sprintf(":%d", config.Port),
		Handler:   mux,
		TLSConfig: server.tlsConfig,
	}

	return server, nil
}

// Start starts the webhook server
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		return s.Shutdown(ctx)
	case err := <-errCh:
		return fmt.Errorf("server error: %v", err)
	}
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleMutate(w http.ResponseWriter, r *http.Request) {
	// Verify content type
	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "invalid Content-Type, want application/json", http.StatusUnsupportedMediaType)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}

	// Parse admission review
	admissionReview := admissionv1.AdmissionReview{}
	if err := json.Unmarshal(body, &admissionReview); err != nil {
		http.Error(w, fmt.Sprintf("failed to decode request: %v", err), http.StatusBadRequest)
		return
	}

	// Process the admission request
	admissionResponse := s.mutate(&admissionReview)
	admissionReview.Response = admissionResponse

	// Send response
	resp, err := json.Marshal(admissionReview)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}
