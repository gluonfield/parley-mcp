package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/gluonfield/parley"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// tenants serves one shared MCP endpoint to many users. Each connection carries
// a bearer token identifying its user; the server holds a separate parley
// identity (and in-flight parley) per user, minted on first use. This is the
// shape a hosted relay's MCP wants — one URL every user adds, each acting as
// themselves — the same way Linear or Slack scope a shared MCP per user.
//
// The trade-off is custody: this server holds every user's parley private key,
// just as a hosted Linear MCP holds each user's OAuth token. It is therefore
// trusted, and belongs behind the same auth boundary as the rest of Jaz.
type tenants struct {
	dir   string
	relay parley.Relay
	host  string

	// userID resolves a request to its authenticated user, or returns false to
	// reject it. The default reads the Bearer token verbatim; a deployment
	// behind Jaz auth swaps in a validator without touching the rest.
	userID func(*http.Request) (string, bool)

	mu      sync.Mutex
	servers map[string]*mcp.Server
}

func newTenants(dir string, relay parley.Relay, host string) *tenants {
	return &tenants{
		dir:     dir,
		relay:   relay,
		host:    host,
		userID:  bearerUser,
		servers: make(map[string]*mcp.Server),
	}
}

// handler is the multi-tenant MCP endpoint: it authenticates the user, then
// lets the streamable transport drive that user's own server.
func (t *tenants) handler() http.Handler {
	stream := mcp.NewStreamableHTTPHandler(t.serverFor, nil)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := t.userID(r); !ok {
			http.Error(w, "missing or invalid bearer token", http.StatusUnauthorized)
			return
		}
		stream.ServeHTTP(w, r)
	})
}

func (t *tenants) serverFor(r *http.Request) *mcp.Server {
	uid, ok := t.userID(r)
	if !ok {
		return nil
	}
	s, err := t.server(uid)
	if err != nil {
		return nil
	}
	return s
}

// server returns the user's MCP server, building it (and minting their identity)
// the first time. One server per user persists their in-flight parley across the
// many requests of a session.
func (t *tenants) server(uid string) (*mcp.Server, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s, ok := t.servers[uid]; ok {
		return s, nil
	}
	self, err := loadIdentity(t.identityPath(uid))
	if err != nil {
		return nil, err
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "parley", Version: "0.1.0"}, nil)
	newAgent(self, t.relay, t.host).register(s)
	t.servers[uid] = s
	return s, nil
}

// identityPath keys a user's key file by the hash of their id, so the token
// itself never lands on disk as a filename.
func (t *tenants) identityPath(uid string) string {
	sum := sha256.Sum256([]byte(uid))
	return filepath.Join(t.dir, hex.EncodeToString(sum[:])+".json")
}

func bearerUser(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || h[:len(prefix)] != prefix {
		return "", false
	}
	return h[len(prefix):], true
}
