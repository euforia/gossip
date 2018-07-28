
NAME = gossip

.PHONY: test
test:
	go test -v -cover ./...
