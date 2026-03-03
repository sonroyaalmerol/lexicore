# syntax=docker/dockerfile:1
FROM golang:1.25-trixie AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags='-w -s -extldflags "-static"' \
  -a \
  -o lexicore \
  ./cmd/controller/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags='-w -s -extldflags "-static"' \
  -a \
  -o lexictl \
  ./cmd/lexictl/

FROM debian:trixie-slim

RUN apt-get update && \
  apt-get install -y --no-install-recommends ca-certificates && \
  rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/lexicore /usr/local/bin/lexicore
COPY --from=builder /build/lexictl /usr/local/bin/lexictl

RUN groupadd -g 65532 nonroot && \
  useradd -u 65532 -g nonroot -s /bin/bash -m nonroot

RUN mkdir -p /etc/lexicore /var/lib/lexicore && \
  chown -R nonroot:nonroot /etc/lexicore /var/lib/lexicore

USER nonroot

EXPOSE 8080

WORKDIR /var/lib/lexicore

ENTRYPOINT ["/usr/local/bin/lexicore"]
CMD ["--config", "/etc/lexicore/config.yaml"]
