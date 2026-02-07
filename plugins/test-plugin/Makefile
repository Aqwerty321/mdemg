.PHONY: build clean test

BINARY_NAME = test-plugin

build:
	go build -o $(BINARY_NAME) .

clean:
	rm -f $(BINARY_NAME)

test:
	go test -v ./...

run: build
	./$(BINARY_NAME) --socket /tmp/$(BINARY_NAME).sock
