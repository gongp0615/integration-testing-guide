package server

import (
	"testing"

	"github.com/example/ai-integration-test-demo/internal/breakpoint"
	"github.com/example/ai-integration-test-demo/internal/event"
	"github.com/example/ai-integration-test-demo/internal/player"
)

func newTestServer(profile string) *Server {
	bus := event.NewBus()
	pm := player.NewManager(bus)
	pm.CreatePlayer(10001)
	bp := breakpoint.NewController(bus)
	return New(pm, bus, bp, profile)
}

func TestL0RequiresRegisteredBusinessCommand(t *testing.T) {
	s := newTestServer("l0")
	state := &sessionState{customCmds: make(map[string]RegisteredCommand)}

	resp := s.dispatch(Request{
		Cmd:      "additem",
		PlayerID: 10001,
		ItemID:   2001,
		Count:    1,
	}, state)
	if resp.Ok {
		t.Fatalf("expected builtin additem to be blocked in l0, got success: %#v", resp)
	}

	reg := s.dispatch(Request{
		Cmd:    "register_cmd",
		Name:   "seed_item",
		Target: "bag",
		Action: "AddItem",
	}, state)
	if !reg.Ok {
		t.Fatalf("register_cmd failed: %#v", reg)
	}

	queued := s.dispatch(Request{
		Cmd:      "seed_item",
		PlayerID: 10001,
		ItemID:   2001,
		Count:    1,
	}, state)
	if !queued.Ok {
		t.Fatalf("expected custom command to queue successfully: %#v", queued)
	}

	s.dispatch(Request{Cmd: "next"}, state)
	bagResp := s.dispatch(Request{Cmd: "playermgr", PlayerID: 10001, Sub: "bag", ItemID: 2001}, state)
	if !bagResp.Ok || bagResp.Data == nil {
		t.Fatalf("expected item to exist after registered add command: %#v", bagResp)
	}
}

func TestRegisteredRawRemoveExposesB1(t *testing.T) {
	s := newTestServer("l1")
	state := &sessionState{customCmds: make(map[string]RegisteredCommand)}

	seed := s.dispatch(Request{Cmd: "additem", PlayerID: 10001, ItemID: 2001, Count: 1}, state)
	if !seed.Ok {
		t.Fatalf("seed additem failed: %#v", seed)
	}
	s.dispatch(Request{Cmd: "next"}, state)

	reject := s.dispatch(Request{Cmd: "removeitem", PlayerID: 10001, ItemID: 2001, Count: -1}, state)
	if reject.Ok {
		t.Fatalf("expected builtin removeitem to reject negative count in l1: %#v", reject)
	}

	reg := s.dispatch(Request{
		Cmd:    "register_cmd",
		Name:   "test_remove_negative",
		Target: "bag",
		Action: "RemoveItem",
		Desc:   "raw negative remove to validate missing business-layer check",
	}, state)
	if !reg.Ok {
		t.Fatalf("register_cmd failed: %#v", reg)
	}

	custom := s.dispatch(Request{
		Cmd:      "test_remove_negative",
		PlayerID: 10001,
		ItemID:   2001,
		Count:    -1,
	}, state)
	if !custom.Ok {
		t.Fatalf("expected custom raw remove command to queue successfully: %#v", custom)
	}
	s.dispatch(Request{Cmd: "next"}, state)

	item := s.pm.GetPlayer(10001).Bag.GetItem(2001)
	if item == nil {
		t.Fatalf("expected item to remain in bag")
	}
	if item.Count != 2 {
		t.Fatalf("expected raw negative remove to reproduce B1 and increase count to 2, got %d", item.Count)
	}
}
