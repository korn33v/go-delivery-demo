FROM golang:1.27.1-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -o /service ./services/fulfillment/cmd/fulfillment && CGO_ENABLED=0 go build -buildvcs=false -trimpath -o /probe ./cmd/probe
FROM alpine:3.24.2
WORKDIR /app
RUN adduser -D -u 10001 app
COPY --from=build /service /app/service
COPY --from=build /probe /probe
COPY services/fulfillment/api /app/api
COPY services/fulfillment/migrations /app/migrations
COPY deploy/swagger /app/swagger
USER app
ENTRYPOINT ["/app/service"]
