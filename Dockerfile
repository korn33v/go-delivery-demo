FROM golang:1.27.1-bookworm AS test
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV GOFLAGS=-buildvcs=false
CMD ["go", "test", "-race", "./..."]
FROM test AS build
RUN CGO_ENABLED=0 go build -trimpath -o /demo ./cmd/demo && CGO_ENABLED=0 go build -trimpath -o /probe ./cmd/probe
FROM alpine:3.24.2 AS tools
COPY --from=build /demo /demo
COPY --from=build /probe /probe
WORKDIR /work
ENTRYPOINT ["/demo"]
