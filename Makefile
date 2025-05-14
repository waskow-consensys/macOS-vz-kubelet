ROOT_DIR := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

RELEASE_CERTIFICATE_NAME ?= ""
RELEASE_PROVISION_PROFILE_PATH ?= ""

include makefiles/go.mk

e2e-test: GOTESTARGS += --namespace e2e $(if $(IMAGE),--image $(IMAGE)) --pod-creation-timeout 10m -exec $(realpath $(dir $(firstword $(MAKEFILE_LIST)))/makefiles/scripts/sign-and-run.sh)

.PHONY: snapshot release

snapshot:
	ROOT_DIR=$(ROOT_DIR) \
	RELEASE_CERTIFICATE_NAME="$(RELEASE_CERTIFICATE_NAME)" \
	RELEASE_PROVISION_PROFILE_PATH="$(RELEASE_PROVISION_PROFILE_PATH)" \
		goreleaser release --clean --snapshot

release:
	ROOT_DIR=$(ROOT_DIR) \
	RELEASE_CERTIFICATE_NAME="$(RELEASE_CERTIFICATE_NAME)" \
	RELEASE_PROVISION_PROFILE_PATH="$(RELEASE_PROVISION_PROFILE_PATH)" \
		goreleaser release --clean
