.PHONY: build run

build:
	go build -trimpath ./cmd/llm-proxy

run: build
	./llm-proxy
