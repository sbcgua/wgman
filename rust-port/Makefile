.PHONY: build test fmt lint check

build:
	cargo build

test:
	cargo test

fmt:
	cargo fmt --check

lint:
	cargo clippy --all-targets -- -D warnings

check: fmt lint test
