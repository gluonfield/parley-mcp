# syntax=docker/dockerfile:1
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/parley-mcp .

# Served over HTTP as one shared, no-auth endpoint: every user adds the same URL
# and supplies a high-entropy Parley-Identity-Seed header. The process derives
# each node key in memory and keeps no at-rest tenant key store.
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/parley-mcp /parley-mcp
EXPOSE 7777
ENTRYPOINT ["/parley-mcp"]
CMD ["--http", "0.0.0.0:7777", "--shared", "--relay", "https://parley.chat"]
