FROM registry.redhat.io/openshift/golang-builder:golang-builder-v1.26-rhel9 AS builder

COPY . /workspace
WORKDIR /workspace/
ENV GOEXPERIMENT strictfipsruntime
RUN CGO_ENABLED=1 GOOS=linux go build -v -mod=vendor -tags strictfipsruntime -o /workspace/bin/hypershift-oadp-plugin .

FROM registry.redhat.io/ubi9/ubi:latest
RUN dnf -y install openssl && dnf -y reinstall tzdata && dnf clean all
RUN mkdir /plugins
COPY --from=builder /workspace/bin/hypershift-oadp-plugin /plugins/
COPY LICENSE /licenses/
USER 65534:65534
ENTRYPOINT ["/bin/bash", "-c", "cp /plugins/* /target/."]

LABEL description="OpenShift API for Data Protection - Hypershift Velero Plugin"
LABEL io.k8s.description="OpenShift API for Data Protection - Hypershift Velero Plugin"
LABEL io.k8s.display-name="OpenShift API for Data Protection Hypershift Velero Plugin"
LABEL io.openshift.tags="migration"
LABEL summary="OpenShift API for Data Protection - Hypershift Velero Plugin"
