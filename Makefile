# The name of Terraform custom provider.
CUSTOM_PROVIDER_NAME ?= terraform-provider-st-tencentcloud
# The url of Terraform provider.
CUSTOM_PROVIDER_URL ?= example.local/myklst/st-tencentcloud

UNAME := $(shell uname)

.PHONY: install-local-custom-provider
install-local-custom-provider: darwin_arm64 linux_amd64

darwin_arm64:
ifneq ($(UNAME), Darwin)
	$(info 'skip darwin_arm64')
else
	export PROVIDER_LOCAL_PATH='$(CUSTOM_PROVIDER_URL)'
	GOOS=darwin GOARCH=arm64 go install .
	GO_INSTALL_PATH="$$(go env GOPATH)/bin"; \
	HOME_DIR="$$(ls -d ~)"; \
	mkdir -p  $$HOME_DIR/.terraform.d/plugins/$(CUSTOM_PROVIDER_URL)/0.1.0/darwin_arm64/; \
	cp $$GO_INSTALL_PATH/$(CUSTOM_PROVIDER_NAME) $$HOME_DIR/.terraform.d/plugins/$(CUSTOM_PROVIDER_URL)/0.1.0/darwin_arm64/$(CUSTOM_PROVIDER_NAME)
endif

linux_amd64:
ifneq ($(UNAME), Linux)
	$(info 'skip linux_amd64')
else
	export PROVIDER_LOCAL_PATH='$(CUSTOM_PROVIDER_URL)'
	GOOS=linux GOARCH=amd64 go install .
	GO_INSTALL_PATH="$$(go env GOPATH)/bin"; \
	HOME_DIR="$$(ls -d ~)"; \
	mkdir -p  $$HOME_DIR/.terraform.d/plugins/$(CUSTOM_PROVIDER_URL)/0.1.0/linux_amd64/; \
	cp $$GO_INSTALL_PATH/$(CUSTOM_PROVIDER_NAME) $$HOME_DIR/.terraform.d/plugins/$(CUSTOM_PROVIDER_URL)/0.1.0/linux_amd64/$(CUSTOM_PROVIDER_NAME)
	unset PROVIDER_LOCAL_PATH
endif

.PHONY: generate-terraform-document
generate-terraform-document:
	go generate ./...

.PHONY: go-fmt
go-fmt:
	goimports -l -w -local "github.com/myklst/$(CUSTOM_PROVIDER_NAME)/" .

.PHONY: go-lint
go-lint:
	golangci-lint run

.PHONY: go-test
go-test:
	go test -v ./...
