package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gluonfield/parley"
	"github.com/gluonfield/parley/relayhttp"
)

func TestTenantIdentitySeedsDeriveStableNodeIDs(t *testing.T) {
	tn := newTenants(nil, "relay.example")

	alice1, err := tn.node(testSeed("alice"))
	if err != nil {
		t.Fatal(err)
	}
	alice2, err := tn.node(testSeed("alice"))
	if err != nil {
		t.Fatal(err)
	}
	bob, err := tn.node(testSeed("bob"))
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

	alice, err := tn.node(testSeed("alice"))
	if err != nil {
		t.Fatal(err)
	}
	bob, err := tn.node(testSeed("bob"))
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

func TestTenantHTTPRequiresIdentitySeed(t *testing.T) {
	tn := newTenants(nil, "relay.example")
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	rr := httptest.NewRecorder()

	tn.handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "missing "+identityHeader) {
		t.Fatalf("body = %q, want missing identity header", rr.Body.String())
	}
}

func TestTenantSessionBindingRejectsIdentitySwitch(t *testing.T) {
	tn := newTenants(nil, "relay.example")
	alice, err := tn.node(testSeed("alice"))
	if err != nil {
		t.Fatal(err)
	}
	bob, err := tn.node(testSeed("bob"))
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
	req.Header.Set(identityHeader, "short")
	if _, err := identitySeed(req); err == nil {
		t.Fatal("short identity seed accepted")
	}

	req.Header.Set(identityHeader, testSeed("alice"))
	got, err := identitySeed(req)
	if err != nil {
		t.Fatal(err)
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
