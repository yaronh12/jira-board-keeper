.PHONY: build test clean docker-build label-sync status-report stale-report

build:
	go build -o bin/jira-board-reporter .

test:
	go test ./... -v -race

clean:
	rm -rf bin/

docker-build:
	docker build -t jira-board-reporter .

label-sync: build
	./bin/jira-board-reporter label-sync --config config.yaml

status-report: build
	./bin/jira-board-reporter status-report --config config.yaml

stale-report: build
	./bin/jira-board-reporter stale-report --config config.yaml
