FROM golang

WORKDIR /app
COPY . .

CMD ./bin/progstack
