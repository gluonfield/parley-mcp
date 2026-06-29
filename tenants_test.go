package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gluonfield/parley"
	"github.com/gluonfield/parley/relayhttp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTenantIdentitySeedsDeriveStableNodeIDs(t *testing.T) {
	tn := newTenants(nil, "relay.example")

	alice1, err := tn.nodeFor(seedRequest(testSeed("alice")))
	if err != nil {
		t.Fatal(err)
	}
	alice2, err := tn.nodeFor(seedRequest(testSeed("alice")))
	if err != nil {
		t.Fatal(err)
	}
	bob, err := tn.nodeFor(seedRequest(testSeed("bob")))
	if err != nil {
		t.Fatal(err)
	}

	if alice1 != alice2 {
		t.Fatal("same identity seed must reuse one node")
	}
	aliceID := nodeID(alice1)
	if aliceID != nodeID(alice2) {
		t.Fatal("same identity seed derived different node ids")
	}
	if aliceID == nodeID(bob) {
		t.Fatal("different identity seeds derived the same node id")
	}
}

func TestTenantNodesIsolateInFlightParleys(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(relayhttp.NewServer().Handler())
	defer srv.Close()
	tn := newTenants(relayhttp.NewClient(srv.URL), "parley.test")

	alice, err := tn.nodeFor(seedRequest(testSeed("alice")))
	if err != nil {
		t.Fatal(err)
	}
	bob, err := tn.nodeFor(seedRequest(testSeed("bob")))
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := alice.agent.open(ctx, nil, openIn{Topic: "alice task"}); err != nil {
		t.Fatalf("alice open: %v", err)
	}
	if _, st, err := bob.agent.status(ctx, nil, statusIn{}); err != nil || st.State != "idle" {
		t.Fatalf("bob status = %+v err=%v, want idle", st, err)
	}
	if _, _, err := alice.agent.open(ctx, nil, openIn{Topic: "second"}); err == nil {
		t.Fatal("same identity was allowed to replace an in-flight parley")
	}
	if _, _, err := bob.agent.open(ctx, nil, openIn{Topic: "bob task"}); err != nil {
		t.Fatalf("different identity did not get its own in-flight parley: %v", err)
	}
}

func TestTenantHTTPAllowsNoIdentitySeed(t *testing.T) {
	tn := newTenants(nil, "relay.example")

	a, err := tn.nodeFor(httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if err != nil {
		t.Fatal(err)
	}
	b, err := tn.nodeFor(httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if err != nil {
		t.Fatal(err)
	}
	if nodeID(a) == nodeID(b) {
		t.Fatal("new requests without identity seeds should not share a node")
	}

	tn.bindSession("session-1", a.key)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set(mcpSessionIDHeader, "session-1")
	got, err := tn.nodeFor(req)
	if err != nil {
		t.Fatal(err)
	}
	if got != a {
		t.Fatal("request with an existing MCP session did not reuse its anonymous node")
	}
}

func TestTenantHTTPConnectsWithoutIdentitySeed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tn := newTenants(nil, "relay.example")
	srv := httptest.NewServer(tn.handler())
	defer srv.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             srv.URL,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) == 0 {
		t.Fatal("no tools returned")
	}
}

func TestTenantSessionBindingRejectsIdentitySwitch(t *testing.T) {
	tn := newTenants(nil, "relay.example")
	alice, err := tn.nodeFor(seedRequest(testSeed("alice")))
	if err != nil {
		t.Fatal(err)
	}
	bob, err := tn.nodeFor(seedRequest(testSeed("bob")))
	if err != nil {
		t.Fatal(err)
	}

	tn.bindSession("session-1", alice.key)
	if err := tn.checkSession("session-1", alice.key); err != nil {
		t.Fatalf("same identity rejected: %v", err)
	}
	if err := tn.checkSession("session-1", bob.key); err == nil {
		t.Fatal("identity switch accepted for an existing MCP session")
	}
}

func TestIdentitySeedHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	if _, ok, err := identitySeed(req); ok || err != nil {
		t.Fatalf("missing seed = ok %v err %v, want optional", ok, err)
	}

	req.Header.Set(identityHeader, "short")
	if _, _, err := identitySeed(req); err == nil {
		t.Fatal("short identity seed accepted")
	}

	req.Header.Set(identityHeader, testSeed("alice"))
	got, ok, err := identitySeed(req)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("identity seed was not reported present")
	}
	if got != testSeed("alice") {
		t.Fatalf("seed = %q", got)
	}
}

func nodeID(node *tenantNode) parley.NodeID {
	return parley.Identity{Key: node.agent.self.Public}.ID()
}

func testSeed(label string) string {
	return label + "-0123456789abcdef0123456789abcdef"
}

func seedRequest(seed string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set(identityHeader, seed)
	return req
}
