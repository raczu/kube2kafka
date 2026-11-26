.ONESHELL:
.SHELLFLAGS = -ec
SHELL = /bin/bash

ifndef VERBOSE
.SILENT:
endif

GINKGO_BASE_FLAGS = -r -cover -race -coverprofile=coverage.out
ifdef CI
GINKGO_FLAGS = $(GINKGO_BASE_FLAGS) --github-output
else
GINKGO_FLAGS = $(GINKGO_BASE_FLAGS) -v -p
endif

K8S_VERSION ?= 1.37
K8S_CLUSTER_NAME ?= dev.kube2kafka.local
KAFKA_NATIVE_VERSION ?= 4.3.2

KAFKA_TEST_BROKER ?= localhost:9092

TIMESTAMP := $(shell date "+%Y%m%d-%H%M%S")

##@ General

# The targets behavior can be customized by setting the following variables:
#
# * Use VERBOSE=1 to enable verbose mode, e.g. make VERBOSE=1 test.
# * Use CI=1 to enable CI mode, e.g. make CI=1 test.
# * Use KAFKA_TEST_BROKER=<broker> to set the broker for integration tests (default:
#   localhost:9092, the "make kafka" container), e.g. make KAFKA_TEST_BROKER=host:9092 test.
#   Integration suites fail fast if the broker is unset or unreachable; "make test-unit"
#   needs no broker.

all: help

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage: make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

start: minikube kafka ## Start development environment

minikube: LOG := /tmp/$(TIMESTAMP)-minikube-start.log
minikube:
	printf "\uea9c Starting Kubernetes cluster"
	minikube start -p $(K8S_CLUSTER_NAME) --kubernetes-version=$(K8S_VERSION) --driver=docker >$(LOG) 2>&1 \
		|| { printf "\nFailed to start the Kubernetes cluster, see $(LOG)\n" >&2; exit 1; }
	printf " \ueab2\n"

kafka: LOG := /tmp/$(TIMESTAMP)-kafka-native-start.log
kafka:
	printf "\uea9c Starting kafka-native container"
	docker run -d \
		--name=kafka \
		-p 9092:9092 \
		-e KAFKA_LISTENERS="CONTROLLER://localhost:9091,HOST://0.0.0.0:9092,DOCKER://0.0.0.0:9093" \
		-e KAFKA_ADVERTISED_LISTENERS="HOST://localhost:9092,DOCKER://kafka:9093" \
		-e KAFKA_LISTENER_SECURITY_PROTOCOL_MAP="CONTROLLER:PLAINTEXT,HOST:PLAINTEXT,DOCKER:PLAINTEXT" \
		-e KAFKA_NODE_ID=1 \
		-e KAFKA_PROCESS_ROLES="broker,controller" \
		-e KAFKA_CONTROLLER_LISTENER_NAMES="CONTROLLER" \
		-e KAFKA_CONTROLLER_QUORUM_VOTERS="1@localhost:9091" \
		-e KAFKA_INTER_BROKER_LISTENER_NAME="DOCKER" \
		-e KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1 \
		apache/kafka-native:$(KAFKA_NATIVE_VERSION) >$(LOG) 2>&1 \
		|| { printf "\nFailed to start kafka-native container, see $(LOG)\n" >&2; exit 1; }
	for _ in $(shell seq 60); do \
		if docker logs kafka 2>&1 | grep -q "Kafka Server started"; then \
			printf " \ueab2\n"; \
			exit 0; \
		fi; \
		sleep 1; \
	done; \
	printf "\nKafka did not start within 60s, see docker logs kafka\n" >&2; \
	exit 1

stop: minikube-stop kafka-stop ## Stop development environment

minikube-stop:
	printf "\uea9c Stopping $(K8S_CLUSTER_NAME) cluster"
	minikube stop -p $(K8S_CLUSTER_NAME) >/dev/null 2>&1 || true
	printf " \ueab2\n"

kafka-stop:
	printf "\uea9c Stopping kafka-native container"
	docker rm -f kafka >/dev/null 2>&1 || true
	printf " \ueab2\n"

status: ## Show status of development environment
	minikube status -p $(K8S_CLUSTER_NAME) || true
	docker ps -f name=^kafka$$ --format "table {{ .ID }}\t{{ .Names }}\t{{ .Status }}\t{{ .Ports }}\t{{ .CreatedAt }}"

lint: ## Run linters and other checks defined in prek.toml
	prek run --all-files

test: ## Run unit and integration tests (needs Kafka)
	KAFKA_TEST_BROKER=$(KAFKA_TEST_BROKER) go tool ginkgo $(GINKGO_FLAGS) --label-filter='!e2e'

test-unit: ## Run unit tests (no external dependencies)
	go tool ginkgo $(GINKGO_FLAGS) --label-filter='!integration && !e2e'

test-integration: ## Run integration tests (needs Kafka)
	KAFKA_TEST_BROKER=$(KAFKA_TEST_BROKER) go tool ginkgo $(GINKGO_FLAGS) --label-filter='integration'

##@ Build

GIT_COMMIT = $(shell git rev-parse --short HEAD)

docker-build: ## Build docker image with the current git commit
	docker build -t kube2kafka:$(GIT_COMMIT) .
