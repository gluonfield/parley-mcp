package main

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gluonfield/parley/noise"
	"github.com/gluonfield/parley/relayhttp"
)

func testAgent(t *testing.T, relay *relayhttp.Client) *agent {
	t.Helper()
	kp, err := noise.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	return newAgent(kp, relay, "parley.test")
}

// TestAgentsTalkThroughTools drives two agents entirely through the MCP tool
// handlers, over a live relay, to confirm the server wiring matches the engine.
func TestAgentsTalkThroughTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv := httptest.NewServer(relayhttp.NewServer().Handler())
	defer srv.Close()
	relay := relayhttp.NewClient(srv.URL)

	a := testAgent(t, relay)
	b := testAgent(t, relay)

	_, opened, err := a.open(ctx, nil, openIn{Topic: "lunch"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, _, err := b.join(ctx, nil, joinIn{Link: opened.Invite}); err != nil {
		t.Fatalf("join: %v", err)
	}

	var (
		wg     sync.WaitGroup
		errs   [2]error
		bReply sayOut
		aHeard recvOut
		aFinal sayOut
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, out, err := b.say(ctx, nil, sayIn{Text: "hi a"})
		if err != nil {
			errs[0] = err
			return
		}
		bReply = out
		_, _, errs[0] = b.close(ctx, nil, closeIn{Outcome: "settled"})
	}()
	go func() {
		defer wg.Done()
		_, heard, err := a.recv(ctx, nil, recvIn{})
		if err != nil {
			errs[1] = err
			return
		}
		aHeard = heard
		_, final, err := a.say(ctx, nil, sayIn{Text: "hi b"})
		if err != nil {
			errs[1] = err
			return
		}
		aFinal = final
	}()
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Fatalf("conversation: %v", err)
		}
	}
	if aHeard.Message != "hi a" {
		t.Errorf("a heard %q", aHeard.Message)
	}
	if bReply.Reply != "hi b" {
		t.Errorf("b heard %q", bReply.Reply)
	}
	if !aFinal.Closed {
		t.Error("a did not observe the close")
	}
	if _, st, _ := a.status(ctx, nil, statusIn{}); st.State != "closed" {
		t.Errorf("status = %q, want closed", st.State)
	}
}
