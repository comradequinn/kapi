# Builds the kapi lib
.PHONY: build
build:
	@export CGO_ENABLED=0; go build

# Tests the kapi lib
.PHONY: test
test:
	@go test -v ./...

#############################
#### Kapi Example Targets ###
#############################

IMAGE=kapi-example
VERSION=v1.0.0
KAPI_EXAMPLE_DIR=cmd/example
KAPI_CLUSTER=kapi
KAPI_NAMESPACE=kapi-example
BUILD_IMAGE=true

# Creates a sandbox k8s cluster for kapi using kind
.PHONY: example-cluster
example-cluster:
	@which kind > /dev/null || (echo "kind not found, installing..." && go install sigs.k8s.io/kind@latest && echo "kind installed successfully")
	@kind get clusters 2> /dev/null | grep -q "${KAPI_CLUSTER}" || ( kind create cluster --name ${KAPI_CLUSTER} && echo "kapi cluster created successfully" )
	@kubectl config use-context kind-${KAPI_CLUSTER}
	@kubectl config set-context --current --namespace ${KAPI_NAMESPACE}

# Creates a kapi cluster using kind and then builds and adds the kapi-example image to its internal container registry.
# Finally it deploys the example controller to it.
.PHONY: example
example: example-cluster
	@REBUILD="${BUILD_IMAGE}"; test "$$REBUILD" = "true" && ( echo "building kapi-example image" && docker build -f ${KAPI_EXAMPLE_DIR}/Dockerfile --tag ${IMAGE}:latest --build-arg VERSION="${VERSION}" . ) || true
	@kind load docker-image ${IMAGE}:latest --name ${KAPI_CLUSTER}
	@kubectl apply -k ${KAPI_EXAMPLE_DIR}/deploy

# Creates a kapi cluster using kind then runs the example controller locally which connects to it.
.PHONY: example-local
example-local: example-cluster
	@kubectl create namespace ${KAPI_NAMESPACE} 2> /dev/null || true
	@kubectl apply -k ${KAPI_EXAMPLE_DIR}/deploy/crd
	@export CGO_ENABLED=0; cd ${KAPI_EXAMPLE_DIR} && go build -o bin/kapi-example && bin/kapi-example | jq

.PHONY: example-adhoc-deployment
example-adhoc-deployment:
	@kubectl apply -k ${KAPI_EXAMPLE_DIR}/deploy/adhoc

# Deletes the kapi cluster and everything in the kapi namespace
.PHONY: delete-example
delete-example:
	@kind delete clusters ${KAPI_CLUSTER} | true

# Preview the kubernetes yaml for the example controller
.PHONY: preview-example-yaml
preview-example-yaml:
	@kubectl kustomize ${KAPI_EXAMPLE_DIR}/deploy


#########################################
###### Kapi Example Observability #######
########################################

.PHONY: logs
logs:
	@kubectl logs "$$(kubectl get pods | awk 'END {print $$1}')" | jq '. | select(.type != "metric")'

.PHONY: logs-summary
logs-summary:
	@kubectl logs "$$(kubectl get pods | awk 'END {print $$1}')" | jq '. | select(.type != null) | select(.type | test("_summary"))'

.PHONY: logs-trace
logs-trace:
	@kubectl logs "$$(kubectl get pods | awk 'END {print $$1}')" | jq '. | select(.type != null) | select(.type | test("_trace"))'

.PHONY: metrics
metrics:
	@kubectl logs "$$(kubectl get pods | awk 'END {print $$1}')" | jq '. | select(.type == "metric")'
