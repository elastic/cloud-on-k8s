FROM buildpack-deps:bullseye-curl AS builder

ARG CLIENT_VERSION

# Kind to run k8s cluster locally in Docker
RUN curl -fsSLO https://github.com/kubernetes-sigs/kind/releases/download/v${CLIENT_VERSION}/kind-linux-amd64 && \
    mv kind-linux-amd64 /usr/local/bin/kind && chmod +x /usr/local/bin/kind

FROM docker:29.2.1-cli

COPY --from=builder /usr/local/bin/kind /usr/local/bin/kind

ENTRYPOINT []
CMD []
