package tui

import (
	"strings"
	"testing"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
)

func TestAssigneePickerOptions_CollapsesAgentSessionsPerHuman(t *testing.T) {
	t.Parallel()

	h1 := "act-human-1"
	h2 := "act-human-2"
	a1 := "act-agent-1"
	a2 := "act-agent-2"
	b1 := "act-agent-3"
	unlinked := "act-agent-unlinked"

	db := &store.DB{
		Actors: []model.Actor{
			{ID: h1, Kind: model.ActorKindHuman, Name: "Andreas"},
			{ID: a1, Kind: model.ActorKindAgent, Name: "[agent-session:codex] Agent", UserID: stringPtr(h1)},
			{ID: a2, Kind: model.ActorKindAgent, Name: "[agent-session:cursor] Agent", UserID: stringPtr(h1)},
			{ID: h2, Kind: model.ActorKindHuman, Name: "Bea"},
			{ID: b1, Kind: model.ActorKindAgent, Name: "[agent-session:codex] Agent", UserID: stringPtr(h2)},
			{ID: unlinked, Kind: model.ActorKindAgent, Name: "[agent-session:service] Agent"},
		},
	}

	opts := assigneePickerOptions(db, "")
	agentForH1 := 0
	for _, o := range opts {
		if strings.TrimSpace(o.id) == "" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(o.label), "agent (Andreas") {
			agentForH1++
		}
	}
	if agentForH1 != 1 {
		t.Fatalf("expected 1 agent option for %s; got %d\nopts=%v", h1, agentForH1, opts)
	}
}

func TestAssigneePickerOptions_PreservesCurrentAssignedAgent(t *testing.T) {
	t.Parallel()

	h1 := "act-human-1"
	a1 := "act-agent-1"
	a2 := "act-agent-2"

	db := &store.DB{
		Actors: []model.Actor{
			{ID: h1, Kind: model.ActorKindHuman, Name: "Andreas"},
			{ID: a1, Kind: model.ActorKindAgent, Name: "[agent-session:codex] Agent", UserID: stringPtr(h1)},
			{ID: a2, Kind: model.ActorKindAgent, Name: "[agent-session:cursor] Agent", UserID: stringPtr(h1)},
		},
	}

	opts := assigneePickerOptions(db, a1)
	found := false
	for _, o := range opts {
		if o.id == a1 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected preferred agent id %s to be present in options; opts=%v", a1, opts)
	}
}

func stringPtr(s string) *string {
	return &s
}
