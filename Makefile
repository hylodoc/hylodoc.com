.PHONY: $(PROGSTACK) $(REPOSITORIES) $(DB) $(BIN) 

DOCKER = $(SUDO) docker
GO = go
BIN = ${CURDIR}/bin
SOURCES := $(shell find $(CURDIR) -name '*.go')
PROGSTACK = $(BIN)/progstack

$(PROGSTACK): $(BIN) $(SOURCES) db get build.sh
	@printf 'BUILD\t$@\n'
	@./build.sh $@

get: go.mod go.sum
	@printf 'GO\tmod tidy\n'
	@go mod tidy

test: get
	@printf 'GO\ttest\n'
	@go test ./... -v

$(BIN):
	@mkdir -p $@

DBDIR = internal/model
dbfiles := $(shell find $(DBDIR) -name '*.sql')
db: $(DBDIR)/sqlc.yaml $(dbfiles)
	@printf 'SQLC\t$<\n'
	@sqlc generate -f $<

up: $(PROGSTACK) test
	@echo 'launching docker containers...'
	$(DOCKER) compose up --build

down:
	@echo 'stopping docker containers...'
	$(DOCKER) compose down

clean:
	@echo 'cleaning up docker resources'
	$(DOCKER) compose down --volumes --remove-orphans
	@rm -rf $(BIN)
	@rm $(DBDIR)/*.gen.go

github:
	@echo 'forwarding Github events to http://localhost:7999/gh/installcallback...'
	smee -u "https://smee.io/D9yWYTiYzjBhfU3O" --port 7999 -P "/gh/installcallback"

stripe:
	@echo 'forwarding Stripe events to http://localhost:7999/stripe/webhook'
	smee -u "https://smee.io/WeoKBRir10gZf0Lf" --port 7999 -P "/stripe/webhook"
