APP_NAME := tisminSRETool
COLLECTOR_NAME := collector-agent
DIST_DIR := dist
GOCACHE ?= $(CURDIR)/.gocache
GO_BUILD_FLAGS ?= -trimpath
VERSION ?= 1.0.0

.PHONY: build build-collector test clean package

build: build-collector

build-collector:
	@mkdir -p $(DIST_DIR)
	GOCACHE=$(GOCACHE) CGO_ENABLED=0 go build $(GO_BUILD_FLAGS) -ldflags "-s -w" -o $(DIST_DIR)/$(COLLECTOR_NAME) ./cmd/collector-agent

test:
	@mkdir -p $(GOCACHE)
	GOCACHE=$(GOCACHE) go test ./...

clean:
	rm -rf $(DIST_DIR)

package: build
	@mkdir -p $(DIST_DIR)/$(APP_NAME)-$(VERSION)/bin/collector-agent $(DIST_DIR)/$(APP_NAME)-$(VERSION)/configs
	cp $(DIST_DIR)/$(COLLECTOR_NAME) $(DIST_DIR)/$(APP_NAME)-$(VERSION)/
	cp bin/collector-agent/start bin/collector-agent/stop bin/collector-agent/restart bin/collector-agent/status $(DIST_DIR)/$(APP_NAME)-$(VERSION)/bin/collector-agent/
	cp configs/collector.yaml $(DIST_DIR)/$(APP_NAME)-$(VERSION)/configs/

