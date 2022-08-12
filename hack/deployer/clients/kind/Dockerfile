FROM docker:20.10-dind

ARG CLIENT_VERSION

RUN apk add --no-cache curl && \
    curl -fsSLO https://github.com/kubernetes-sigs/kind/releases/download/v${CLIENT_VERSION}/kind-linux-amd64 && \
    mv kind-linux-amd64 /usr/local/bin/kind && chmod +x /usr/local/bin/kind \
