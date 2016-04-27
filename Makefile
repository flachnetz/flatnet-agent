VERSION?=$(shell awk -F\" '/^const Version/ { print $$2; exit }' version.go)
VETARGS?=-asmdecl -atomic -bool -buildtags -copylocks -methods \
         -nilfunc -printf -rangeloops -shift -structtags -unsafeptr
DOCKER_GOLANG_BUILDER_IMAGE?=devservices01.office.tipp24.de:5000/iwg/golang-builder:1.6
RELEASE_VERSION?=$$(cat __RELEASE_VERSION__ || echo $(VERSION))
BINARY=flatnet
REMAP_UID?=$${UID}


all						: update-dependencies format test build

$(BINARY)				: *.go ./lib/*.go
						@$(MAKE) build

tools					:
						@if [ "$$(which glide)" == "" ]; then \
							go get github.com/Masterminds/glide; \
						else \
							echo Glide is available at $$(which glide) ;\
						fi

update-dependencies		:
						@glide install

test					:
						@go test -v flatnet flatnet

format					:
						@go fmt flatnet flatnet/lib/...

vet						:
						@go tool vet -v $(VETARGS) lib/*.go *.go

build					:
						@scripts/build.sh $(VERSION) linux amd64
						@scripts/build.sh $(VERSION) darwin amd64

docker-compile-test		:
						@docker run --rm --entrypoint=sh -v $$(pwd):/go/src/flatnet:Z $(DOCKER_GOLANG_BUILDER_IMAGE) \
						-c "apt-get install libpcap-dev -y; cd flatnet; make update-dependencies build test"

docker-build			:
						@docker run --rm --entrypoint=sh -v $$(pwd):/go/src/flatnet:Z $(DOCKER_GOLANG_BUILDER_IMAGE) \
						-c "apt-get install libpcap-dev -y; cd flatnet; make update-dependencies build"

clean					:
						rm __RELEASE_VERSION__ || true
						rm -rf flatnet* || true

setup-ide				:
						@scripts/setup-ide.sh

.PHONY					:clean docker-compile-test

