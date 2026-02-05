# Makefile Parser Test Fixture
# Tests symbol extraction for Makefiles
# Line numbers are predictable for UPTS validation

# === Pattern 1: Variable assignments ===
# Line 7-10: Various assignment operators
CC = gcc
CFLAGS := -Wall -Wextra -O2
PREFIX ?= /usr/local
LDFLAGS += -lpthread

# Line 12: Multi-line variable with backslash continuation
SOURCES = main.c \
	utils.c \
	parser.c

# Line 16-17: More variables
VERSION := 1.2.3
BUILD_DIR := build

# === Pattern 2: Export statement ===
# Line 20
export PATH

# === Pattern 3: .PHONY declaration ===
# Line 23
.PHONY: all clean install test lint help

# === Pattern 4: Targets ===
# Line 26-27: Default target
all: $(BUILD_DIR)/app
	@echo "Build complete"

# Line 29-31: Build target with recipe lines
$(BUILD_DIR)/app: $(SOURCES)
	@mkdir -p $(BUILD_DIR)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $^

# Line 33-35: Clean target
clean:
	rm -rf $(BUILD_DIR)
	rm -f *.o

# Line 37-39: Install target
install: all
	install -d $(PREFIX)/bin
	install -m 755 $(BUILD_DIR)/app $(PREFIX)/bin/

# Line 41-42: Test target
test:
	./run_tests.sh

# Line 44-45: Lint target
lint:
	cppcheck --enable=all $(SOURCES)

# === Pattern 5: Pattern rule ===
# Line 48-49: Pattern rule for object files
%.o: %.c
	$(CC) $(CFLAGS) -c -o $@ $<

# === Pattern 6: define/endef macro ===
# Line 52-56
define run_checks
	@echo "Running checks..."
	@$(1) --check $(2)
	@echo "Done."
endef

# === Pattern 7: Include statement ===
# Line 59
include config.mk

# === Pattern 8: Target with semicolon recipe ===
# Line 62
help: ; @echo "Available targets: all clean install test lint help"

# === Pattern 9: Conditional variable ===
# Line 65
DEBUG_FLAGS ?= -g -DDEBUG

# === Pattern 10: Another export with value ===
# Line 68
export GOPATH = $(HOME)/go

# === Pattern 11: Complex target ===
# Line 71-74
release: clean all
	@echo "Creating release v$(VERSION)"
	tar czf app-$(VERSION).tar.gz -C $(BUILD_DIR) app
	@echo "Release package created"

# Line 76-78: Docker target
docker:
	docker build -t myapp:$(VERSION) .
	docker tag myapp:$(VERSION) myapp:latest

# Line 80-82: Deploy target (not in .PHONY)
deploy: release docker
	docker push myapp:$(VERSION)
	docker push myapp:latest
