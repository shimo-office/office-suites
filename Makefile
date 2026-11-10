.PHONY: test lint

test:
	./cli/tests/smoke.sh

lint:
	python3 -m py_compile cli/bin/shimo.py
	@echo "py_compile ok"
