FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . /app

RUN apk add --no-cache wget tar

RUN wget https://downloads.sqlc.dev/sqlc_1.27.0_linux_amd64.tar.gz -O /tmp/sqlc.tar.gz && \
    tar -xzf /tmp/sqlc.tar.gz -C /tmp/ && \
    mv /tmp/sqlc /usr/local/bin/ && \
    rm /tmp/sqlc.tar.gz
RUN sqlc generate -f internal/model/sqlc.yaml

RUN ls -l /app
RUN ls -l /app/web

RUN go build -o progstack
CMD ./progstack
