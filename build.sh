#!/bin/sh

# static: echo "1" if build should be static
static() {
	for arg in "$@"; do
		case $arg in
			--static)
				echo 1
				;;
		esac
	done
	if [ "$(uname)" = "Darwin" ]; then
		echo 1
	fi
}

if [ -z "$1" ]; then
    echo "error: no binary name provided"
    echo "usage: $0 <binary_name>"
    exit 1
fi
BINARY=$1

if [ "$(uname)" = "Darwin" ]; then
	CC=x86_64-linux-musl-gcc
fi

if [ "$(static "$@")" = "1" ]; then
	LDFLAGS="-a -ldflags=-extldflags=-static"
fi

GOOS=linux
GOARCH=amd64

set -x

CGO_ENABLED=1 GOOS=$GOOS GOARCH=$GOARCH CC=$CC \
    time go build $LDFLAGS -o $BINARY
