.PHONY: all build clean run check cover lint docker help

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
