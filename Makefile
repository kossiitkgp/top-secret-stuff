DATABASE_VOLUME := food

ifeq (, $(shell which docker-compose))
    DOCKER_COMPOSE=docker compose
else
    DOCKER_COMPOSE=docker-compose
endif

CURRENT_MAKEFILE := $(lastword $(MAKEFILE_LIST))

PROJECT_DIR := $(shell dirname $(realpath $(CURRENT_MAKEFILE)))
TUMMY_CONTAINER_ID = $(shell docker ps -q --filter "name=tummy" --filter "status=running")

default: build run

.PHONY: help build run stop digest run-digester check_clean clean

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@sed -n 's/^##//p' $(CURRENT_MAKEFILE) | column -t -s ':' |  sed -e 's/^/ /'
	@echo ""
	@echo "Running 'make' without a target is equivalent to running 'make build run'."

## build: Build the excretor and tummy docker images
build:
	@echo "Building excretor and tummy docker images..."
	@ZIPFILE_PATH=. $(DOCKER_COMPOSE) build excretor tummy

## run: Run the excretor and tummy docker containers
run:
	@echo "Running excretor and tummy docker containers..."
	@ZIPFILE_PATH=. $(DOCKER_COMPOSE) up excretor tummy -d

## stop: Stop the excretor and tummy docker containers
stop:
	@echo "Stopping excretor and tummy docker containers..."
	@ZIPFILE_PATH=. $(DOCKER_COMPOSE) stop excretor tummy
	@ZIPFILE_PATH=. $(DOCKER_COMPOSE) down excretor tummy

## digest: Run the digester container
digest:
ifeq (, $(FILE))
	@echo "ERROR: No file path provided. Please specify the file path using 'make digest FILE=/path-to-file'"
	@exit 1;
endif
ifneq (, $(TUMMY_CONTAINER_ID))
	@echo "Tummy container is already running."
	@$(MAKE) run-digester --no-print-directory;
else
	@echo "Starting tummy container..."
	@ZIPFILE_PATH=. $(DOCKER_COMPOSE) up tummy -d;
	@$(MAKE) run-digester --no-print-directory;
	@ZIPFILE_PATH=. $(DOCKER_COMPOSE) down;
endif
	@echo "Digestion complete."

run-digester:
	@echo ""
	@echo "Building and running digester container..."
	@ZIPFILE_PATH=$(FILE) $(DOCKER_COMPOSE) up digester --build --abort-on-container-exit;
	@ZIPFILE_PATH=$(FILE) $(DOCKER_COMPOSE) down digester;

check_clean:
	@echo "This will remove the database volume. This action is irreversible."
	@echo -n "Are you sure you want to proceed? [y/N] " && read ans; \
    if [ $${ans:-N} != y ] && [ $${ans:-N} != Y ]; then \
        echo "Operation canceled."; \
        exit 1; \
    fi

## clean: Remove the database volume
clean: check_clean
	@docker volume rm $(notdir $(PROJECT_DIR))_$(DATABASE_VOLUME)
	@echo "Database volume removed."

%:
	@echo "Target '$(MAKECMDGOALS)' not found."
	@echo ""
	@$(MAKE) --no-print-directory help