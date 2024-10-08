.ONESHELL:
.SHELLFLAGS = -ec
SHELL = /bin/bash

ifndef VERBOSE
.SILENT:
endif

K8S_VERSION ?= 1.30
K8S_CLUSTER_NAME ?= dev.kube2kafka.local
KAFKA_NATIVE_VERSION ?= 3.8.0

help:
	echo "Usage: make <target>"
	echo ""
	echo "Available targets:"
	echo "  help          Show this help message"
	echo "  start         Start development environment"
	echo "  stop          Stop development environment"
	echo "  status        Show status of development environment"
	echo ""
	echo "Use VERBOSE=1 to enable verbose mode, e.g. VERBOSE=1 make start"

all: help

start:
	command -v minikube >/dev/null || (printf "\uea9c minikube is not installed"; exit 1)
	printf "\uea9c Starting Kubernetes cluster"
	minikube start -p $(K8S_CLUSTER_NAME) --kubernetes-version=$(K8S_VERSION) --driver=docker >/tmp/$$(date "+%Y%m%d%-H%M%S")-minikube-start.log 2>&1
	printf " \ueab2\n"

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
		apache/kafka-native:$(KAFKA_NATIVE_VERSION) >/tmp/$$(date "+%Y%m%d%-H%M%S")-kafka-native-start.log 2>&1
	printf " \ueab2\n"

stop:
	printf "\uea9c Stopping $(K8S_CLUSTER_NAME) cluster"
	minikube stop -p $(K8S_CLUSTER_NAME) >/dev/null 2>&1 || true
	printf " \ueab2\n"

	printf "\uea9c Stopping kafka-native container"
	docker ps -q -f name=kafka -f ancestor=apache/kafka-native:$(KAFKA_NATIVE_VERSION) | xargs -r docker stop >/dev/null 2>&1 || true
	docker ps -aq -f name=kafka -f ancestor=apache/kafka-native:$(KAFKA_NATIVE_VERSION) | xargs -r docker rm >/dev/null 2>&1 || true
	printf " \ueab2\n"

status:
	minikube status -p $(K8S_CLUSTER_NAME) || true
	docker ps -f name=kafka -f ancestor=apache/kafka-native:$(KAFKA_NATIVE_VERSION) --format "table {{ .ID }}\t{{ .Names }}\t{{ .Status }}\t{{ .Ports }}\t{{ .CreatedAt }}"
