// Command parley-mcp runs a parley client as an MCP server over stdio, so any
// MCP host — Claude Code, jaz, others — can let its agent talk to another
// person's agent. It holds this node's identity key, does all the crypto, and
// reaches a relay; the host only ever sees the parley_* tools.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"net/url"

	"github.com/gluonfield/parley"
	"github.com/gluonfield/parley/relayhttp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	relayURL := flag.String("relay", "https://parley.chat", "relay base URL")
	host := flag.String("host", "", "relay host for invite links (default: from -relay)")
	identity := flag.String("identity", defaultIdentityPath(), "node identity file (single-tenant)")
	httpAddr := flag.String("http", "", "serve MCP over HTTP at this address (e.g. 127.0.0.1:7777) instead of stdio")
	shared := flag.Bool("shared", false, "serve one shared no-auth HTTP endpoint; optional Parley identity seed headers give stable nodes")
	tenantsDir := flag.String("tenants", "", "deprecated alias for -shared; value ignored")
	flag.Parse()

	inviteHost := *host
	if inviteHost == "" {
		if u, err := url.Parse(*relayURL); err == nil {
			inviteHost = u.Host
		}
	}
	relay := relayhttp.NewClient(*relayURL)

	if *httpAddr != "" && (*shared || *tenantsDir != "") {
		mux := http.NewServeMux()
		mux.Handle("/mcp", newTenants(relay, inviteHost).handler())
		log.Printf("parley-mcp: shared no-auth MCP endpoint http://%s/mcp on relay %s; identity header %s", *httpAddr, *relayURL, identityHeader)
		if err := http.ListenAndServe(*httpAddr, mux); err != nil {
			log.Fatal(err)
		}
		return
	}

	self, err := loadIdentity(*identity)
	if err != nil {
		log.Fatal(err)
	}
	a := newAgent(self, relay, inviteHost)
	server := mcp.NewServer(&mcp.Implementation{Name: "parley", Version: "0.1.0"}, nil)
	a.register(server)
	log.Printf("parley-mcp: node %s on relay %s", parley.Identity{Key: self.Public}.Fingerprint(), *relayURL)

	if *httpAddr != "" {
		mux := http.NewServeMux()
		mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
		log.Printf("parley-mcp: MCP endpoint http://%s/mcp", *httpAddr)
		if err := http.ListenAndServe(*httpAddr, mux); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
