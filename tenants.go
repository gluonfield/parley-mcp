package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gluonfield/parley"
	"github.com/gluonfield/parley/noise"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	identityHeader      = "Parley-Identity-Seed"
	minIdentitySeedSize = 32
	mcpSessionIDHeader  = "Mcp-Session-Id"
)

// tenants serves one shared, no-auth MCP endpoint to many users. Each
// connection presents a self-asserted identity seed in identityHeader; equal
// seeds derive the same parley node, and different seeds derive different
// nodes. The seed is not a login credential and is not validated against an
// account.
//
// Custody tradeoff: while handling requests, this hosted process is the parley
// node for that seed and holds the derived private key material in memory so it
// can run the Noise handshake. That differs from the local-node threat model
// where the private key never leaves the user's machine.
type tenants struct {
	relay parley.Relay
	host  string

	mu       sync.Mutex
	nodes    map[string]*tenantNode
	sessions map[string]string
}

type tenantNode struct {
	key    string
	server *mcp.Server
	agent  *agent
}

type tenantNodeContextKey struct{}

func newTenants(relay parley.Relay, host string) *tenants {
	return &tenants{
		relay:    relay,
		host:     host,
		nodes:    make(map[string]*tenantNode),
		sessions: make(map[string]string),
	}
}

func (t *tenants) handler() http.Handler {
	stream := mcp.NewStreamableHTTPHandler(t.serverFor, nil)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seed, err := identitySeed(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		node, err := t.node(seed)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sessionID := r.Header.Get(mcpSessionIDHeader)
		if err := t.checkSession(sessionID, node.key); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		r = r.WithContext(context.WithValue(r.Context(), tenantNodeContextKey{}, node))
		rw := &statusWriter{ResponseWriter: w}
		stream.ServeHTTP(rw, r)

		if newSessionID := rw.Header().Get(mcpSessionIDHeader); newSessionID != "" {
			t.bindSession(newSessionID, node.key)
		}
		if r.Method == http.MethodDelete && sessionID != "" && rw.status == http.StatusNoContent {
			t.unbindSession(sessionID)
		}
	})
}

func (t *tenants) serverFor(r *http.Request) *mcp.Server {
	if node, ok := r.Context().Value(tenantNodeContextKey{}).(*tenantNode); ok {
		return node.server
	}
	seed, err := identitySeed(r)
	if err != nil {
		return nil
	}
	node, err := t.node(seed)
	if err != nil {
		return nil
	}
	return node.server
}

// node returns the MCP server for seed, deriving the parley identity on demand
// without writing key material to disk.
func (t *tenants) node(seed string) (*tenantNode, error) {
	self, err := noise.DeriveKeypair([]byte(seed))
	if err != nil {
		return nil, err
	}
	id := parley.Identity{Key: self.Public}.ID()
	key := hex.EncodeToString(id[:])

	t.mu.Lock()
	defer t.mu.Unlock()
	if node, ok := t.nodes[key]; ok {
		return node, nil
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "parley", Version: "0.1.0"}, nil)
	a := newAgent(self, t.relay, t.host)
	a.register(s)
	node := &tenantNode{key: key, server: s, agent: a}
	t.nodes[key] = node
	return node, nil
}

func (t *tenants) checkSession(sessionID, nodeKey string) error {
	if sessionID == "" {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	owner, ok := t.sessions[sessionID]
	if !ok {
		return nil
	}
	if owner != nodeKey {
		return fmt.Errorf("MCP session belongs to a different Parley identity")
	}
	return nil
}

func (t *tenants) bindSession(sessionID, nodeKey string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessions[sessionID] = nodeKey
}

func (t *tenants) unbindSession(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.sessions, sessionID)
}

func identitySeed(r *http.Request) (string, error) {
	seed := strings.TrimSpace(r.Header.Get(identityHeader))
	if seed == "" {
		return "", fmt.Errorf("missing %s header", identityHeader)
	}
	if len(seed) < minIdentitySeedSize {
		return "", fmt.Errorf("%s must be a high-entropy seed of at least %d bytes", identityHeader, minIdentitySeedSize)
	}
	return seed, nil
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
