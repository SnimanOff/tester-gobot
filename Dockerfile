FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/tester-bot ./bot/tester

FROM golang:1.27-alpine
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 10001 childbot \
    && adduser -D -u 10001 -G childbot -h /var/lib/childbot childbot \
    && mkdir -p /var/lib/childbot/go /var/lib/childbot/cache /app/data/children \
    && chown -R childbot:childbot /var/lib/childbot

ENV CGO_ENABLED=0 \
    HOME=/var/lib/childbot \
    GOPATH=/var/lib/childbot/go \
    GOCACHE=/var/lib/childbot/cache \
    CHILDREN_DIR=/app/data/children \
    CHILD_RUN_UID=10001 \
    CHILD_RUN_GID=10001

WORKDIR /app
COPY --from=build /out/tester-bot /app/tester-bot
CMD ["/app/tester-bot"]
