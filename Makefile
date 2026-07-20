BINARY := nuage
CMD := ./cmd/nuage
INSTALL_PATH := $(HOME)/.local/bin/$(BINARY)

.PHONY: build install test vet clean run

build:
	go build -trimpath -ldflags="-s -w" -o $(BINARY) $(CMD)

install: build
	install -Dm755 $(BINARY) $(INSTALL_PATH)

test:
	go test ./...

vet:
	go vet ./...

run: build
	./$(BINARY) serve

clean:
	rm -f $(BINARY)
