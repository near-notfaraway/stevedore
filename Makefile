.PHONY: all prepare build package clean test help

WORK_DIR=./output
OUTPUT_DIR=${WORK_DIR}/stevedore
BIN_DIR=${OUTPUT_DIR}/bin
LOG_DIR=${OUTPUT_DIR}/log
ETC_DIR=${OUTPUT_DIR}/etc
BIN="stevedore"

all: test prepare build package

prepare: clean
	mkdir -p ${WORK_DIR}

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ${BIN}

package:
	mkdir -p ${OUTPUT_DIR} ${BIN_DIR} ${LOG_DIR} ${ETC_DIR}
	mv ${BIN} ${BIN_DIR}
	cp -a sd_config/config.example.* ${ETC_DIR}
	tar -zcf ${OUTPUT_DIR}.tgz ${OUTPUT_DIR}

clean:
	@if [ -d ${WORK_DIR} ]; then rm -rf ${WORK_DIR}; fi

test:
	go test -v -cover ./sd_util

help:
	@echo "Usage:"
	@echo "    make [child command]\n"
	@echo "All child command are as follows:"
	@echo "    make:           The same as make all"
	@echo "    make all:       Run make test -> make prepare -> make build -> make package"
	@echo "    make prepare:   Remake the work directory named 'output'"
	@echo "    make build:     Compile all codes and output the binary file 'stevedore'"
	@echo "    make package:   Pack all files and output the compressed file 'stevedore.tgz'"
	@echo "    make clean:     Remove the work directory named 'output'"
	@echo "    make test:      Run all unit test of the project"
	@echo "    make help:      Show help\n"
