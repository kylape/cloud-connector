CONNECTOR_SERVICE_BINARY=cloud-connector
CONNECTED_CLIENT_BINARY=test_client
MIGRATE_DB_BINARY=migrate_db

DOCKER_COMPOSE_CFG=dev.yml

COVERAGE_OUTPUT=coverage.out
COVERAGE_HTML=coverage.html

.PHONY: test clean deps coverage 

build:
	go build -o $(CONNECTOR_SERVICE_BINARY) cmd/$(CONNECTOR_SERVICE_BINARY)/*.go
	go build -o $(CONNECTED_CLIENT_BINARY) cmd/$(CONNECTED_CLIENT_BINARY)/*.go
	go build -o $(MIGRATE_DB_BINARY) cmd/$(MIGRATE_DB_BINARY)/main.go

deps:
	go get -u golang.org/x/lint/golint

test:
	# Use the following command to run specific tests (not the entire suite)
	# TEST_ARGS="-run TestReadMessage -v" make test
	go test $(TEST_ARGS) ./...

migrate: $(MIGRATE_DB_BINARY)
	./$(MIGRATE_DB_BINARY) upgrade

coverage:
	go test -v -coverprofile=$(COVERAGE_OUTPUT) ./...
	go tool cover -html=$(COVERAGE_OUTPUT) -o $(COVERAGE_HTML)
	@echo "file://$(PWD)/$(COVERAGE_HTML)"

start-test-env:
	podman-compose -f $(DOCKER_COMPOSE_CFG) up

stop-test-env:
	podman-compose -f $(DOCKER_COMPOSE_CFG) down

fmt:
	go fmt ./...

lint:
	$(GOPATH)/bin/golint ./...

clean:
	go clean
	rm -f $(CONNECTOR_SERVICE_BINARY) $(CONNECTED_CLIENT_BINARY) $(MIGRATE_DB_BINARY)
	rm -f $(COVERAGE_OUTPUT) $(COVERAGE_HTML)
