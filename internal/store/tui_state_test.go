package store

import (
        "reflect"
        "testing"
)

func TestTUIState_SaveLoad_RoundTrip(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        s := Store{Dir: dir}

        // Missing file => default state.
        st0, err := s.LoadTUIState()
        if err != nil {
                t.Fatalf("LoadTUIState: %v", err)
        }
        if st0 == nil || st0.Version != 1 {
                t.Fatalf("expected default Version=1; got %#v", st0)
        }

        want := &TUIState{
                Version:           1,
                View:              "item",
                SelectedProjectID: "proj-1",
                SelectedOutlineID: "out-1",
                OpenItemID:        "item-1",
                ReturnView:        "outline",
                AgendaReturnView:  "projects",
                Pane:              "outline",
                ShowPreview:       true,
                OutlineViewMode:   map[string]string{"out-1": "columns"},
        }

        if err := s.SaveTUIState(want); err != nil {
                t.Fatalf("SaveTUIState: %v", err)
        }

        got, err := s.LoadTUIState()
        if err != nil {
                t.Fatalf("LoadTUIState (after save): %v", err)
        }

        if !reflect.DeepEqual(want, got) {
                t.Fatalf("roundtrip mismatch:\nwant: %#v\ngot:  %#v", want, got)
        }
}
