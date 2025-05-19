# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
ARG TARGETOS
ARG TARGETARCH
COPY bin/secretless-webhook-${TARGETOS}-${TARGETARCH} /bin/secretless-webhook
USER 65532:65532

ENTRYPOINT ["/bin/secretless-webhook"]