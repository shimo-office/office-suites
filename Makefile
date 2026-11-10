.PHONY: build test lint clean

build:
	go build -trimpath -ldflags "-s -w" -o bin/shimo-net ./cli

test: build
	go test ./cli
	go vet ./cli
	./bin/shimo-net version
	./bin/shimo-net schema >/dev/null
	./bin/shimo-net config show >/dev/null

lint:
	go vet ./cli

clean:
	rm -rf bin
