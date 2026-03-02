# syntax=docker/dockerfile:1

FROM golang:1.26-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags='-s -w' -o /out/audistro-provider ./cmd/audistro-provider

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app

ENV PROVIDER_DATA_PATH=/var/lib/audistro-provider
VOLUME ["/var/lib/audistro-provider"]
EXPOSE 8080

COPY --from=build /out/audistro-provider /usr/local/bin/audistro-provider
COPY ops /app/ops
CMD ["/usr/local/bin/audistro-provider"]
