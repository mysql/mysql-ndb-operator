#!/usr/bin/env bash

# Copyright (c) 2021, 2023, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Script to generate Ndb CRD and the release artifact

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_GEN_INPUT_PATH="./pkg/apis/..."
HELM_CHART_PATH="deploy/charts/ndb-operator"
CRD_GEN_OUTPUT="${HELM_CHART_PATH}/crds"
CRD_FULL_PATH="${CRD_GEN_OUTPUT}/mysql.oracle.com_ndbclusters.yaml"
CONTROLLER_GEN_CMD="go run sigs.k8s.io/controller-tools/cmd/controller-gen"

# Generate Ndb CRD
echo "Generating Ndb CRD..."
${CONTROLLER_GEN_CMD} "crd" paths=${CRD_GEN_INPUT_PATH} output:crd:artifacts:config=${CRD_GEN_OUTPUT}
# creationTimestamp in the CRD is always generated as null
# https://github.com/kubernetes-sigs/controller-tools/issues/402
# remove it as a workaround
sed -i.crd.bak "/\ \ creationTimestamp\:\ null/d" ${CRD_GEN_OUTPUT}/* && rm ${CRD_GEN_OUTPUT}/*.crd.bak

# Generate a single ndb-operator yaml file for deploying the CRD and the ndb operator in namespace 'ndb-operator'
INSTALL_ARTIFACT="deploy/manifests/ndb-operator.yaml"
echo "Generating install artifact..."
# Copy in the Ndb CRD
cp ${CRD_FULL_PATH} ${INSTALL_ARTIFACT}
# Copy yaml to create 'ndb-operator' namespace
echo "---
apiVersion: v1
kind: Namespace
metadata:
  name: ndb-operator
---" >> ${INSTALL_ARTIFACT}
# Generate and append the resources from helm templates
helm template ndb-operator ${HELM_CHART_PATH} --namespace=ndb-operator >> ${INSTALL_ARTIFACT}
# Prettify the yaml file
go run hack/prettify-yaml.go --yaml=${INSTALL_ARTIFACT}
