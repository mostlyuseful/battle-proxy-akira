.PHONY: build run

build:
	go build ./cmd/llm-proxy

run: build
	./llm-proxy
