BIN = bin
REPOSITORIES = repositories
PROGSTACK = $(BIN)/progstack

DB = internal/model
COMPOSE_FILE = docker-compose.yml

.PHONY: $(PROGSTACK) $(REPOSITORIES) $(DB) $(BIN) 

run: $(PROGSTACK) 
	@echo 'running $(PROGSTACK)...'
	@./$(PROGSTACK)

$(PROGSTACK): $(DB) $(REPOSITORIES) $(BIN)
	@echo 'fetching go dependencies...'
	@go get
	@echo 'building $(PROGSTACK)...'
	@go build -o $@

$(DB): $(DB)/sqlc.yaml
	@echo 'generating SQLC files in $(DB)...'
	@sqlc generate -f $(DB)/sqlc.yaml

$(BIN):
	@echo 'generating bin directory...'
	@mkdir -p $@

$(REPOSITORIES):
	@echo 'generating repositories directory...'
	@mkdir -p $@

# Docker compose targets
up:
	@echo 'starting docker containers in detached mode...'
	docker-compose -f $(COMPOSE_FILE) up -d

down:
	@echo 'stopping docker containers...'
	docker-compose -f $(COMPOSE_FILE) down

docker-clean:
	@echo 'cleaning up docker resources'
	docker-compose -f $(COMPOSE_FILE) down --volumes --remove-orphans

smee:
	@echo 'forwarding Github events to http://localhost:7999/gh/installcallback...'
	smee -u "https://smee.io/D9yWYTiYzjBhfU3O" --port 7999 -P "/gh/installcallback"

clean:
	@echo "removing bin directory..."
	@rm -rf $(BIN)
	@echo "removing all .gen.go files in $(DB)..."
	@rm -f $(DB)/*.gen.go
