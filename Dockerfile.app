FROM golang

# pandoc
RUN apt update && apt install -y pandoc
RUN pandoc --version

WORKDIR /app
COPY . .

CMD ./bin/progstack
