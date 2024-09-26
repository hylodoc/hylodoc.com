.PHONY: $(PROGSTACK) $(REPOSITORIES) $(DB) $(BIN) 

# Docker resource management
up:
	@echo 'launching docker containers...'
	@docker-compose up --build

down:
	@echo 'stopping docker containers...'
	docker-compose down

clean:
	@echo 'cleaning up docker resources'
	docker-compose down --volumes --remove-orphans

smee:
	@echo 'forwarding Github events to http://localhost:7999/gh/installcallback...'
	smee -u "https://smee.io/D9yWYTiYzjBhfU3O" --port 7999 -P "/gh/installcallback"
