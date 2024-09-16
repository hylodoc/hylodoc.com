BIN = bin
PROGSTACK = $(BIN)/progstack

DB = internal/model
COMPOSE_FILE = docker-compose.yml

.PHONY:

run: $(PROGSTACK)
	@./$(PROGSTACK)

$(PROGSTACK): $(BIN) $(DB)
	@printf 'GO\tget\n'
	@go get
	@printf 'GO\tbuild\n'
	@go build -o $@

$(DB): $(DB)/sqlc.yaml
	@printf 'SQLC\t$<\n'
	@sqlc generate -f $<

$(BIN):
	@mkdir -p $@

# Docker compose targets
# Start containers in detached mode
up:
	docker-compose -f $(COMPOSE_FILE) up -d
# Stop and remove containers
down:
	docker-compose -f $(COMPOSE_FILE) down
# Clean up Docker-related resources
docker-clean:
	docker-compose -f $(COMPOSE_FILE) down --volumes --remove-orphans

clean:
	@rm -rf $(BIN)
