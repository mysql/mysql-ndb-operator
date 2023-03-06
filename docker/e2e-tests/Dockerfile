# Copyright (c) 2021, 2023, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
ARG GOLANG_IMAGE=golang:1.20
FROM $GOLANG_IMAGE

# install kubectl
ARG KUBE_VERSION=v1.21.6
RUN curl -LO "https://dl.k8s.io/release/$KUBE_VERSION/bin/linux/amd64/kubectl" && \
      install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# install helm
ARG HELM_VERSION=v3.7.1
RUN curl -LO https://get.helm.sh/helm-$HELM_VERSION-linux-amd64.tar.gz && \
      tar -zxf helm-v3.7.1-linux-amd64.tar.gz && \
      install -o root -g root -m 0755 linux-amd64/helm /usr/bin/helm

# Create and move into the target repo directory
WORKDIR /ndb-operator-e2e-testing/

# Copy the required files/directories in project directory to workdir.
COPY . .

ENTRYPOINT ["docker/e2e-tests/entrypoint.sh"]
CMD ["go", "run", "github.com/onsi/ginkgo/ginkgo", "-r", "-keep-going", "e2e-tests/suites"]
