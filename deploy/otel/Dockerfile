FROM golang:1.27.1-bookworm AS probe
WORKDIR /src
COPY go.mod ./
COPY cmd/probe ./cmd/probe
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -o /probe ./cmd/probe
FROM otel/opentelemetry-collector-contrib:0.161.0
COPY --from=probe /probe /probe
