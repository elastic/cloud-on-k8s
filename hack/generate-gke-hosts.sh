#!/bin/bash

# This script takes responsibility to create the necessary hosts file entries
# for the stack controller to be able to target the default sample deployment.

SVCPREFIX=$(grep "name:" config/samples/deployments_v1alpha1_stack.yaml | sed 's/name://')
SUDOMESSAGE="Please enter your sudo password to modify the hosts file tee"

if ! grep "${SVCPREFIX}-es-public" /etc/hosts > /dev/null || ! grep "${SVCPREFIX}-kb" /etc/hosts > /dev/null; then
    echo "# BEGIN section for kubectl stack operators" | sudo -p "${SUDOMESSAGE}" tee -a /etc/hosts > /dev/null
    echo "127.0.0.1   ${SVCPREFIX}-es-public ${SVCPREFIX}-kb" | sudo -p "${SUDOMESSAGE}" tee -a /etc/hosts > /dev/null
    echo "# END section for kubectl stack operators" | sudo -p "${SUDOMESSAGE}" tee -a /etc/hosts > /dev/null
fi


