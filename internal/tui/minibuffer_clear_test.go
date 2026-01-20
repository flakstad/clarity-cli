package tui

import (
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
)

func TestUpdate_ReloadTickMsg_AutoClearsMinibuffer(t *testing.T) {
	db := &store.DB{
		CurrentActorID: "act-test",
		Actors:         []model.Actor{{ID: "act-test", Kind: model.ActorKindHuman, Name: "tester"}},
	}
	m := newAppModel(t.TempDir(), db)

	(&m).showMinibuffer("Hello")
	m.minibufferSetAt = time.Now().Add(-minibufferAutoClearAfter - 100*time.Millisecond)

	mm, _ := m.Update(reloadTickMsg{})
	m = mm.(appModel)

	if got := m.minibufferText; got != "" {
		t.Fatalf("expected minibuffer text to clear, got %q", got)
	}
}

func TestUpdate_ReloadTickMsg_DoesNotClearRecentMinibuffer(t *testing.T) {
	db := &store.DB{
		CurrentActorID: "act-test",
		Actors:         []model.Actor{{ID: "act-test", Kind: model.ActorKindHuman, Name: "tester"}},
	}
	m := newAppModel(t.TempDir(), db)

	(&m).showMinibuffer("Hello")
	m.minibufferSetAt = time.Now()

	mm, _ := m.Update(reloadTickMsg{})
	m = mm.(appModel)

	if got := m.minibufferText; got == "" {
		t.Fatalf("expected minibuffer text to remain set")
	}
}
