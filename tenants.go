package main

import (
	"context"
	"encoding/hex"
	"errors"
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

var errSessionIdentityMismatch = errors.New("MCP session belongs to a different Parley identity")

// tenants serves one shared, no-auth MCP endpoint to many users. Each
// connection may present a self-asserted identity seed in identityHeader; equal
// seeds derive the same parley node, and different seeds derive different
// nodes. If no seed is presented, the request gets a fresh session-scoped node.
// The seed is not a login credential and is not validated against an account.
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

type requestNode struct {
	node *tenantNode
	err  error
}

type requestNodeContextKey struct{}

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
		if err := t.checkRequestIdentity(r); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errSessionIdentityMismatch) {
				status = http.StatusForbidden
			}
			http.Error(w, err.Error(), status)
			return
		}
		sessionID := r.Header.Get(mcpSessionIDHeader)

		reqNode := new(requestNode)
		r = r.WithContext(context.WithValue(r.Context(), requestNodeContextKey{}, reqNode))
		rw := &statusWriter{ResponseWriter: w}
		stream.ServeHTTP(rw, r)

		if newSessionID := rw.Header().Get(mcpSessionIDHeader); newSessionID != "" && reqNode.node != nil {
			t.bindSession(newSessionID, reqNode.node.key)
		}
		if r.Method == http.MethodDelete && sessionID != "" && rw.status == http.StatusNoContent {
			t.unbindSession(sessionID)
		}
	})
}

func (t *tenants) serverFor(r *http.Request) *mcp.Server {
	if reqNode, ok := r.Context().Value(requestNodeContextKey{}).(*requestNode); ok {
		if reqNode.node == nil && reqNode.err == nil {
			reqNode.node, reqNode.err = t.nodeFor(r)
		}
		if reqNode.err != nil {
			return nil
		}
		return reqNode.node.server
	}
	node, err := t.nodeFor(r)
	if err != nil {
		return nil
	}
	return node.server
}

func (t *tenants) nodeFor(r *http.Request) (*tenantNode, error) {
	if seed, ok, err := identitySeed(r); err != nil {
		return nil, err
	} else if ok {
		self, err := noise.DeriveKeypair([]byte(seed))
		if err != nil {
			return nil, err
		}
		return t.node(self)
	}

	if sessionID := r.Header.Get(mcpSessionIDHeader); sessionID != "" {
		if node := t.nodeForSession(sessionID); node != nil {
			return node, nil
		}
	}

	self, err := noise.GenerateKeypair()
	if err != nil {
		return nil, err
	}
	return t.node(self)
}

func (t *tenants) checkRequestIdentity(r *http.Request) error {
	seed, ok, err := identitySeed(r)
	if err != nil {
		return err
	}
	sessionID := r.Header.Get(mcpSessionIDHeader)
	if sessionID == "" {
		return nil
	}
	t.mu.Lock()
	owner, mapped := t.sessions[sessionID]
	t.mu.Unlock()
	if !mapped || !ok {
		return nil
	}
	key, err := nodeKeyForSeed(seed)
	if err != nil {
		return err
	}
	if owner != key {
		return errSessionIdentityMismatch
	}
	return nil
}

// node returns the MCP server for self, building it on first use.
func (t *tenants) node(self noise.Keypair) (*tenantNode, error) {
	key := nodeKey(self)

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

func nodeKeyForSeed(seed string) (string, error) {
	self, err := noise.DeriveKeypair([]byte(seed))
	if err != nil {
		return "", err
	}
	return nodeKey(self), nil
}

func nodeKey(self noise.Keypair) string {
	id := parley.Identity{Key: self.Public}.ID()
	return hex.EncodeToString(id[:])
}

func (t *tenants) nodeForSession(sessionID string) *tenantNode {
	t.mu.Lock()
	defer t.mu.Unlock()
	key, ok := t.sessions[sessionID]
	if !ok {
		return nil
	}
	return t.nodes[key]
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
		return errSessionIdentityMismatch
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

func identitySeed(r *http.Request) (string, bool, error) {
	seed := strings.TrimSpace(r.Header.Get(identityHeader))
	if seed == "" {
		return "", false, nil
	}
	if len(seed) < minIdentitySeedSize {
		return "", false, fmt.Errorf("%s must be a high-entropy seed of at least %d bytes", identityHeader, minIdentitySeedSize)
	}
	return seed, true, nil
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
