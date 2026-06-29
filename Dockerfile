# syntax=docker/dockerfile:1
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/parley-mcp .

# Served over HTTP, so it has a URL a remote MCP host can reach. The node's
# identity keypair lives under /data — mount a volume there to keep it stable
# across restarts, or the container mints a fresh identity each run.
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/parley-mcp /parley-mcp
EXPOSE 7777
VOLUME ["/data"]
ENTRYPOINT ["/parley-mcp"]
CMD ["--http", "0.0.0.0:7777", "--identity", "/data/identity.json", "--relay", "https://parley.chat"]
