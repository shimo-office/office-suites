.PHONY: build test lint docs-check clean

build:
	go build -trimpath -ldflags "-s -w" -o bin/shimo-net ./cli

test: docs-check build
	go test ./cli
	go vet ./cli
	./bin/shimo-net version
	./bin/shimo-net schema >/dev/null
	./bin/shimo-net config show >/dev/null

docs-check:
	python3 scripts/generate_capabilities.py --check
	python3 -m unittest discover -s scripts -p "test_*.py"
	python3 scripts/check_docs.py

lint: docs-check
	go vet ./cli

clean:
	rm -rf bin
