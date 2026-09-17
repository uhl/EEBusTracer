FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=docker

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /out/eebustracer ./cmd/eebustracer

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/eebustracer /usr/local/bin/eebustracer

USER nonroot:nonroot
VOLUME ["/data"]
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/eebustracer"]
CMD ["serve", "--db", "/data/traces.db"]
