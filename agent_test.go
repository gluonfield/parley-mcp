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

// pollMsg drives one agent's poll tool until a message arrives or the parley closes.
func pollMsg(ctx context.Context, a *agent) (text string, closed bool, err error) {
	for {
		_, out, err := a.poll(ctx, nil, pollIn{WaitSeconds: 2})
		if err != nil {
			return "", false, err
		}
		if len(out.Messages) > 0 {
			return out.Messages[0], out.Closed, nil
		}
		if out.Closed {
			return "", true, nil
		}
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
	}
}

// TestAgentsTalkThroughTools drives two agents through the MCP tool handlers,
// over a live relay, using the non-blocking send/poll surface.
func TestAgentsTalkThroughTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
		wg      sync.WaitGroup
		errs    [2]error
		aHeard  string
		bHeard  string
		aClosed bool
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, _, err := b.send(ctx, nil, sendIn{Text: "hi a"}); err != nil {
			errs[0] = err
			return
		}
		reply, _, err := pollMsg(ctx, b)
		if err != nil {
			errs[0] = err
			return
		}
		bHeard = reply
		_, _, errs[0] = b.close(ctx, nil, closeIn{Outcome: "settled"})
	}()
	go func() {
		defer wg.Done()
		heard, _, err := pollMsg(ctx, a)
		if err != nil {
			errs[1] = err
			return
		}
		aHeard = heard
		if _, _, err := a.send(ctx, nil, sendIn{Text: "hi b"}); err != nil {
			errs[1] = err
			return
		}
		_, closed, err := pollMsg(ctx, a)
		if err != nil {
			errs[1] = err
			return
		}
		aClosed = closed
	}()
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Fatalf("conversation: %v", err)
		}
	}
	if aHeard != "hi a" {
		t.Errorf("a heard %q", aHeard)
	}
	if bHeard != "hi b" {
		t.Errorf("b heard %q", bHeard)
	}
	if !aClosed {
		t.Error("a did not observe the close")
	}
	if _, st, _ := a.status(ctx, nil, statusIn{}); st.State != "closed" || !st.PeerPresent {
		t.Errorf("status = %+v, want closed + present", st)
	}
}
