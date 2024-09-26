# Stage 1: Build the Go application
FROM golang:1.23-alpine AS builder

# set the working directory
WORKDIR /app

# copy go.mod and go.sum
COPY go.mod go.sum ./

# download dependencies
RUN go mod download

# copy the rest of your application code
COPY . .

# install necessary tools
RUN apk add --no-cache wget tar

# download the sqlc tarball
RUN wget https://downloads.sqlc.dev/sqlc_1.27.0_linux_amd64.tar.gz -O /tmp/sqlc.tar.gz && \
    tar -xzf /tmp/sqlc.tar.gz -C /tmp/ && \
    mv /tmp/sqlc /usr/local/bin/ && \
    rm /tmp/sqlc.tar.gz

# run sqlc generation
RUN sqlc generate -f internal/model/sqlc.yaml

# build application
RUN go build -o progstack

# check for application
RUN ls -l /app

# Stage 2: create a minimal image with only the binary
FROM alpine:latest

WORKDIR /app

# copy the built binary from the builder image
COPY --from=builder /app/progstack /usr/local/bin/progstack

# copy configuration
COPY conf.yaml /app/conf.yaml

# expose the port your application runs on
# XXX: should read from .env var
EXPOSE 7999

# command to run the application
CMD ["progstack"]
