.PHONY: build run

build:
	go build -v -x -o bin/switchboard main.go

run:
	$(PWD)/bin/switchboard --image test-image --redis redis://localhost:6379/0 --channel channel-1

clean:
	rm -rf bin