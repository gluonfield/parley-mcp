# syntax=docker/dockerfile:1
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/parley-mcp .

# Served over HTTP as one shared, multi-tenant endpoint: every user adds the same
# URL and the server holds a separate parley identity per bearer token, under
# /data/identities — mount a volume there to keep users' keys across restarts.
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/parley-mcp /parley-mcp
EXPOSE 7777
VOLUME ["/data"]
ENTRYPOINT ["/parley-mcp"]
CMD ["--http", "0.0.0.0:7777", "--tenants", "/data/identities", "--relay", "https://parley.chat"]
