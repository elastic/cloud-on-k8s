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

FROM scratch
# be able to make TLS requests to the GCloud API
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/local/bin/openshift-install .
COPY --from=builder /usr/local/bin/oc .
