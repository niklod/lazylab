.PHONY: build lint test test-e2e clean

build:
	uv sync

lint:
	uvx ruff check --select I --fix .
	uvx ruff check --fix .
	uvx pyright

test:
	uv run pytest tests/unit/ -v

test-e2e:
	uv run pytest tests/e2e/ -v

clean:
	rm -rf .venv build dist *.egg-info
	find . -type d -name __pycache__ -exec rm -rf {} +
