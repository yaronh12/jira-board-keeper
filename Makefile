.PHONY: build test clean docker-build label-sync status-report stale-report

build:
	go build -o bin/jira-board-keeper .

test:
	go test ./... -v -race

clean:
	rm -rf bin/

docker-build:
	docker build -t jira-board-keeper .

label-sync: build
	./bin/jira-board-keeper label-sync --config config.yaml

status-report: build
	./bin/jira-board-keeper status-report --config config.yaml

stale-report: build
	./bin/jira-board-keeper stale-report --config config.yaml
