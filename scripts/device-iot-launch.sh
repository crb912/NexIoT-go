#!/usr/bin/env bash
###
# Launches all EdgeX Go binaries (must be previously built).
#
# Expects that Consul and MongoDB are already installed and running.
#
###

DIR=$PWD
BIN=../bin

function cleanup {
	pkill next-iot-go
}

cd $BIN
exec -a next-iot-go ./next-iot-go &
cd $DIR


trap cleanup EXIT

while : ; do sleep 1 ; done