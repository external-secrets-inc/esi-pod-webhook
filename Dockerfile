# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
ARG TARGETOS
ARG TARGETARCH
COPY bin/esi-pod-webhook-${TARGETOS}-${TARGETARCH} /bin/esi-pod-webhook
USER 65532:65532

ENTRYPOINT ["/bin/esi-pod-webhook"]