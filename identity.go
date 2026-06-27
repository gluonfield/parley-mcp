package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gluonfield/parley/noise"
)

// storedIdentity is the on-disk form of a node's static keypair.
type storedIdentity struct {
	Private string `json:"private"`
	Public  string `json:"public"`
}

// defaultIdentityPath is where a node keeps its key absent an override.
func defaultIdentityPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".parley/identity.json"
	}
	return filepath.Join(home, ".parley", "identity.json")
}

// loadIdentity reads the node's keypair from path, minting and persisting a new
// one the first time. The private key is written 0600 under a 0700 directory.
func loadIdentity(path string) (noise.Keypair, error) {
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		return decodeIdentity(b)
	case !errors.Is(err, os.ErrNotExist):
		return noise.Keypair{}, fmt.Errorf("parley-mcp: read identity: %w", err)
	}

	kp, err := noise.GenerateKeypair()
	if err != nil {
		return noise.Keypair{}, err
	}
	enc := base64.RawURLEncoding
	data, err := json.MarshalIndent(storedIdentity{
		Private: enc.EncodeToString(kp.Private),
		Public:  enc.EncodeToString(kp.Public[:]),
	}, "", "  ")
	if err != nil {
		return noise.Keypair{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return noise.Keypair{}, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return noise.Keypair{}, fmt.Errorf("parley-mcp: write identity: %w", err)
	}
	return kp, nil
}

func decodeIdentity(b []byte) (noise.Keypair, error) {
	var st storedIdentity
	if err := json.Unmarshal(b, &st); err != nil {
		return noise.Keypair{}, fmt.Errorf("parley-mcp: parse identity: %w", err)
	}
	enc := base64.RawURLEncoding
	priv, err := enc.DecodeString(st.Private)
	if err != nil {
		return noise.Keypair{}, fmt.Errorf("parley-mcp: parse identity: %w", err)
	}
	pub, err := enc.DecodeString(st.Public)
	if err != nil || len(pub) != len(noise.Keypair{}.Public) {
		return noise.Keypair{}, fmt.Errorf("parley-mcp: parse identity: bad public key")
	}
	kp := noise.Keypair{Private: priv}
	copy(kp.Public[:], pub)
	return kp, nil
}
