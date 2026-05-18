FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /jira-board-keeper .

FROM alpine:3.20
RUN apk --no-cache add ca-certificates
COPY --from=builder /jira-board-keeper /usr/local/bin/jira-board-keeper
ENTRYPOINT ["jira-board-keeper"]
