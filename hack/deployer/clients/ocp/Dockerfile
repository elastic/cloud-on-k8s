FROM buildpack-deps:bullseye-curl as builder
ARG CLIENT_VERSION


# OpenShift installer and CLI to provision OCP clusters
RUN OCP_MAJOR="${CLIENT_VERSION%%.*}" && \
    BASE_URL="https://mirror.openshift.com/pub/openshift-v${OCP_MAJOR}/clients/ocp/${CLIENT_VERSION}" && \
    curl -fsSLO ${BASE_URL}/openshift-install-linux-${CLIENT_VERSION}.tar.gz && \
    tar -zxf openshift-install-linux-${CLIENT_VERSION}.tar.gz openshift-install && \
    mv openshift-install /usr/local/bin/openshift-install && \
    rm openshift-install-linux-${CLIENT_VERSION}.tar.gz && \
    curl -fsSLO ${BASE_URL}/openshift-client-linux-${CLIENT_VERSION}.tar.gz && \
    tar -zxf openshift-client-linux-${CLIENT_VERSION}.tar.gz oc && \
    mv oc /usr/local/bin/oc && \
    rm openshift-client-linux-${CLIENT_VERSION}.tar.gz

FROM registry.access.redhat.com/ubi9/ubi-minimal:9.9-1795001629
COPY --from=builder /usr/local/bin/openshift-install .
COPY --from=builder /usr/local/bin/oc .
