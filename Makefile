.PHONY: all prepare build linux darwin build-linux build-darwin package clean test test-race help

WORK_DIR=./output
OUTPUT_DIR=${WORK_DIR}/stevedore
BIN_DIR=${OUTPUT_DIR}/bin
LOG_DIR=${OUTPUT_DIR}/log
ETC_DIR=${OUTPUT_DIR}/etc
BIN="stevedore"
BUILD_OS?=$(shell go env GOOS)
DARWIN_ARCH?=$(shell go env GOARCH)

all: test prepare build package

prepare: clean
	mkdir -p ${WORK_DIR}

build: ${BUILD_OS}

linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ${BIN}

darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=${DARWIN_ARCH} go build -o ${BIN}

build-linux: linux

build-darwin: BIN="stevedore-darwin-${DARWIN_ARCH}"
build-darwin: darwin

package:
	mkdir -p ${OUTPUT_DIR} ${BIN_DIR} ${LOG_DIR} ${ETC_DIR}
	mv ${BIN} ${BIN_DIR}
	cp -a sd_config/config.example.* ${ETC_DIR}
	tar -zcf ${OUTPUT_DIR}.tgz ${OUTPUT_DIR}

clean:
	@if [ -d ${WORK_DIR} ]; then rm -rf ${WORK_DIR}; fi

test:
	go test -v -cover ./...

test-race:
	go test -race ./...

help:
	@echo "Usage:"
	@echo "    make [child command]\n"
	@echo "All child command are as follows:"
	@echo "    make:           The same as make all"
	@echo "    make all:       Run make test -> make prepare -> make build -> make package"
	@echo "    make prepare:   Remake the work directory named 'output'"
	@echo "    make build:     Build 'stevedore' for the current system"
	@echo "    make linux:     Build the Linux amd64 binary 'stevedore'"
	@echo "    make darwin:    Build 'stevedore' for \$${DARWIN_ARCH}"
	@echo "    make build-darwin: Build 'stevedore-darwin-\$${DARWIN_ARCH}'"
	@echo "    make package:   Pack all files and output the compressed file 'stevedore.tgz'"
	@echo "    make clean:     Remove the work directory named 'output'"
	@echo "    make test:      Run all unit test of the project"
	@echo "    make test-race: Run all unit tests with the race detector"
	@echo "    make help:      Show help\n"
