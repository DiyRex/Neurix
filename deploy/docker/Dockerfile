FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/prometheus/common/version.Version=${VERSION} -extldflags '-static'" \
    -o /ollama_exporter ./cmd/ollama_exporter/

FROM scratch
COPY --from=builder /ollama_exporter /ollama_exporter
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
EXPOSE 9400
ENTRYPOINT ["/ollama_exporter"]
