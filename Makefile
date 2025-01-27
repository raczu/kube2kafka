.ONESHELL:
.SHELLFLAGS = -ec
SHELL = /bin/bash

ifndef VERBOSE
.SILENT:
endif

GINKGO_BASE_FLAGS = -r -cover -race
ifdef CI
GINKGO_FLAGS = $(GINKGO_BASE_FLAGS) --github-output
else
GINKGO_FLAGS = $(GINKGO_BASE_FLAGS) -v -coverprofile=coverage.out -p
endif

K8S_VERSION ?= 1.30
K8S_CLUSTER_NAME ?= dev.kube2kafka.local
KAFKA_NATIVE_VERSION ?= 3.8.0

KAFKA_TEST_BROKER ?= localhost:9092

##@ General

# The targets behavior can be customized by setting the following variables:
#
# * Use VERBOSE=1 to enable verbose mode, e.g. make VERBOSE=1 test.
# * Use CI=1 to enable CI mode, e.g. make CI=1 test.
# * Use KAFKA_TEST_BROKER=<broker> to set the broker for tests (required for tests), e.g.
#   make KAFKA_TEST_BROKER=localhost:9092 test.

all: help

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage: make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

start: minikube kafka ## Start development environment

minikube:
	printf "\uea9c Starting Kubernetes cluster"
	minikube start -p $(K8S_CLUSTER_NAME) --kubernetes-version=$(K8S_VERSION) --driver=docker >/tmp/$(shell date "+%Y%m%d-%H%M%S")-minikube-start.log 2>&1
	printf " \ueab2\n"

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
		apache/kafka-native:$(KAFKA_NATIVE_VERSION) >/tmp/$(shell date "+%Y%m%d-%H%M%S")-kafka-native-start.log 2>&1
	printf " \ueab2\n"

stop: minikube-stop kafka-stop ## Stop development environment

minikube-stop:
	printf "\uea9c Stopping $(K8S_CLUSTER_NAME) cluster"
	minikube stop -p $(K8S_CLUSTER_NAME) >/dev/null 2>&1 || true
	printf " \ueab2\n"

kafka-stop:
	printf "\uea9c Stopping kafka-native container"
	docker ps -q -f name=kafka -f ancestor=apache/kafka-native:$(KAFKA_NATIVE_VERSION) | xargs -r docker stop >/dev/null 2>&1 || true
	docker ps -aq -f name=kafka -f ancestor=apache/kafka-native:$(KAFKA_NATIVE_VERSION) | xargs -r docker rm >/dev/null 2>&1 || true
	printf " \ueab2\n"

status: ## Show status of development environment
	minikube status -p $(K8S_CLUSTER_NAME) || true
	docker ps -f name=kafka -f ancestor=apache/kafka-native:$(KAFKA_NATIVE_VERSION) --format "table {{ .ID }}\t{{ .Names }}\t{{ .Status }}\t{{ .Ports }}\t{{ .CreatedAt }}"

lint: golangci-lint tidy-lint ## Run linters

golangci-lint:
	golangci-lint run

tidy-lint:
	go mod tidy && git diff --exit-code go.mod go.sum

test: ## Run tests
	KAFKA_TEST_BROKER=$(KAFKA_TEST_BROKER) ginkgo $(GINKGO_FLAGS)

##@ Build

GIT_COMMIT = $(shell git rev-parse --short HEAD)

docker-build: ## Build docker image with the current git commit
	docker build -t kube2kafka:$(GIT_COMMIT) .
