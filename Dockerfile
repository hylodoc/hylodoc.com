FROM ubuntu:22.04

RUN apt-get update && \
    apt-get install -y ca-certificates && \
    update-ca-certificates

WORKDIR /app
COPY . .

CMD ./bin/progstack
