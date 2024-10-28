FROM golang:1.23-alpine AS builder

# sqlc
RUN apk add --no-cache tar
ADD https://downloads.sqlc.dev/sqlc_1.27.0_linux_amd64.tar.gz /tmp/sqlc.tar.gz
RUN tar -xzf /tmp/sqlc.tar.gz -C /tmp/ && \
    mv /tmp/sqlc /usr/local/bin/ && \
    rm /tmp/sqlc.tar.gz

# gcc for Cgo
RUN apk add gcc musl-dev

WORKDIR /app
COPY . .

# generate models with sqlc
RUN sqlc generate -f internal/model/sqlc.yaml

# Run unit tests
RUN go test ./... -v

# build application
RUN CGO_ENABLED=1 go build -o progstack
CMD ./progstack
