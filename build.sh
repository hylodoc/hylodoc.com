#!/bin/sh

if [ -z "$1" ]; then
    echo "error: no binary name provided"
    echo "usage: $0 <binary_name>"
    exit 1
fi
BINARY=$1


GOOS=linux
GOARCH=amd64

if [ "$(uname)" = "Darwin" ]; then
	CC=x86_64-linux-musl-gcc
	LDFLAGS="-a -ldflags=-extldflags=-static"
fi

set -x

CGO_ENABLED=1 GOOS=$GOOS GOARCH=$GOARCH CC=$CC \
    time go build $LDFLAGS -o $BINARY
