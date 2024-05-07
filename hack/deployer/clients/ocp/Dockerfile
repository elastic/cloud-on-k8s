FROM buildpack-deps:bullseye-curl as builder
ARG CLIENT_VERSION


# OpenShift installer and CLI to provision OCP clusters
RUN curl -fsSLO https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${CLIENT_VERSION}/openshift-install-linux-${CLIENT_VERSION}.tar.gz && \
    tar -zxf openshift-install-linux-${CLIENT_VERSION}.tar.gz openshift-install && \
    mv openshift-install /usr/local/bin/openshift-install && \
    rm openshift-install-linux-${CLIENT_VERSION}.tar.gz && \
    curl -fsSLO https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${CLIENT_VERSION}/openshift-client-linux-${CLIENT_VERSION}.tar.gz && \
    tar -zxf openshift-client-linux-${CLIENT_VERSION}.tar.gz oc && \
    mv oc /usr/local/bin/oc && \
    rm openshift-client-linux-${CLIENT_VERSION}.tar.gz

FROM registry.access.redhat.com/ubi9/ubi-minimal:9.4-949.1714662671
COPY --from=builder /usr/local/bin/openshift-install .
COPY --from=builder /usr/local/bin/oc .
