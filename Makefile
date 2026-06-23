export PATH := $(PATH):`go env GOPATH`/bin
export GO111MODULE=on
export CGO_ENABLED=0
LDFLAGS := -s -w
NOWEB_TAG := ,noweb
ifneq ($(wildcard web/frps/dist),)
ifneq ($(wildcard web/frpc/dist),)
NOWEB_TAG :=
endif
endif
FRP_COMPAT_BASELINE_COUNT ?= 8
FRP_COMPAT_FLOOR_VERSION ?= 0.61.0

# Windows .exe suffix
ifeq ($(OS),Windows_NT)
BIN_EXT := .exe
else
BIN_EXT :=
endif

.PHONY: web frps-web frpc-web frps frpc e2e-compatibility-smoke e2e-compatibility e2e-compatibility-floor

all: env fmt web build

build: frps frpc

env:
	@go version

web: frps-web frpc-web

frps-web:
	$(MAKE) -C web/frps build

frpc-web:
	$(MAKE) -C web/frpc build

fmt:
	go fmt ./...

fmt-more:
	gofumpt -l -w .

gci:
	gci write -s standard -s default -s "prefix(github.com/fatedier/frp/)" ./

vet:
	go vet -tags "$(NOWEB_TAG)" ./...

frps:
	go build -trimpath -ldflags "$(LDFLAGS)" -tags "frps$(NOWEB_TAG)" -o bin/frps$(BIN_EXT) ./cmd/frps

frpc:
	go build -trimpath -ldflags "$(LDFLAGS)" -tags "frpc$(NOWEB_TAG)" -o bin/frpc$(BIN_EXT) ./cmd/frpc

test: gotest

gotest:
	go test -tags "$(NOWEB_TAG)" -v --cover ./assets/...
	go test -tags "$(NOWEB_TAG)" -v --cover ./cmd/...
	go test -tags "$(NOWEB_TAG)" -v --cover ./client/...
	go test -tags "$(NOWEB_TAG)" -v --cover ./server/...
	go test -tags "$(NOWEB_TAG)" -v --cover ./pkg/...

e2e:
	./hack/run-e2e.sh

e2e-trace:
	DEBUG=true LOG_LEVEL=trace ./hack/run-e2e.sh

e2e-compatibility-smoke: build
	FRP_COMPAT_BASELINE_COUNT=1 ./hack/run-e2e-compatibility.sh

e2e-compatibility: build
	FRP_COMPAT_BASELINE_COUNT="$(FRP_COMPAT_BASELINE_COUNT)" ./hack/run-e2e-compatibility.sh

e2e-compatibility-floor: build
	FRP_COMPAT_BASELINE_VERSIONS="$(FRP_COMPAT_FLOOR_VERSION)" ./hack/run-e2e-compatibility.sh

e2e-compatibility-last-frpc:
ifeq ($(OS),Windows_NT)
	if not exist "./lastversion" (set TARGET_DIRNAME=lastversion && .\hack\download.sh)
	set FRPC_PATH="%cd%\lastversion\frpc" && .\hack\run-e2e.sh
	if exist "./lastversion" rmdir /s /q .\lastversion
else
	if [ ! -d "./lastversion" ]; then \
		TARGET_DIRNAME=lastversion ./hack/download.sh; \
	fi
	FRPC_PATH="`pwd`/lastversion/frpc" ./hack/run-e2e.sh
	rm -r ./lastversion
endif

e2e-compatibility-last-frps:
ifeq ($(OS),Windows_NT)
	if not exist "./lastversion" (set TARGET_DIRNAME=lastversion && .\hack\download.sh)
	set FRPS_PATH="%cd%\lastversion\frps" && .\hack\run-e2e.sh
	if exist "./lastversion" rmdir /s /q .\lastversion
else
	if [ ! -d "./lastversion" ]; then \
		TARGET_DIRNAME=lastversion ./hack/download.sh; \
	fi
	FRPS_PATH="`pwd`/lastversion/frps" ./hack/run-e2e.sh
	rm -r ./lastversion
endif

alltest: vet gotest e2e
	
clean:
ifeq ($(OS),Windows_NT)
	if exist .\bin\frpc.exe del /q .\bin\frpc.exe
	if exist .\bin\frps.exe del /q .\bin\frps.exe
	if exist .\lastversion rmdir /s /q .\lastversion
	if exist .\.cache rmdir /s /q .\.cache
	if exist .\.compat rmdir /s /q .\.compat
else
	rm -f ./bin/frpc
	rm -f ./bin/frps
	rm -rf ./lastversion
	rm -rf ./.cache
	rm -rf ./.compat
endif
