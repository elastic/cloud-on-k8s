# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

#
# Generate single YAML to deploy the operator.
#

export SHELL := /bin/bash

ifndef NAMESPACE
$(error NAMESPACE not set)
endif
ifndef OPERATOR_IMAGE
$(error OPERATOR_IMAGE not set)
endif

generate-all-in-one:
	@ for yaml in $$(LC_COLLATE=C ls all-in-one/*yaml); do \
		printf "\n---\n" && cat $$yaml ; \
	done | \
	sed \
		-e "/# /d" \
		-e "s|<OPERATOR_IMAGE>|$$OPERATOR_IMAGE|g" \
		-e "s|<NAMESPACE>|$$NAMESPACE|g"

generate-namespace:
ifndef MANAGED_NAMESPACES
	$(error MANAGED_NAMESPACES not set)
endif
	@ for yaml in $(shell find namespace -type f -name '*.yaml' ! -name 'managed_ns_role_binding.template.yaml'); do \
		printf "\n---\n" && cat $$yaml ; \
	done | \
	sed \
		-e "/# /d" \
		-e "s|<OPERATOR_IMAGE>|$$OPERATOR_IMAGE|g" \
		-e "s|<NAMESPACE>|$$NAMESPACE|g" \
		-e "s|<MANAGED_NAMESPACE_LIST>|$$MANAGED_NAMESPACES|g"

	@ (MANAGED_NS_LIST=$${MANAGED_NAMESPACES//,/ }; \
	  for managed_ns in $$MANAGED_NS_LIST; do \
		printf "\n---\n" && sed \
		-e "/# /d" \
		-e "s|<NAMESPACE>|$$NAMESPACE|g" \
		-e "s|<MANAGED_NAMESPACE>|$$managed_ns|g" \
		namespace/managed_ns_role_binding.template.yaml ;\
	  done)
