package statusutil

import (
        "testing"

        "clarity-cli/internal/model"
)

func TestNormalizeStatusID(t *testing.T) {
        cases := []struct {
                in      string
                want    string
                wantErr bool
        }{
                {"todo", "todo", false},
                {"TODO", "todo", false},
                {"doing", "doing", false},
                {"DONE", "done", false},
                {"none", "", false},
                {"  backlog ", "backlog", false},
                {"", "", true},
                {"   ", "", true},
        }
        for _, tc := range cases {
                got, err := NormalizeStatusID(tc.in)
                if tc.wantErr && err == nil {
                        t.Fatalf("NormalizeStatusID(%q): expected error", tc.in)
                }
                if !tc.wantErr && err != nil {
                        t.Fatalf("NormalizeStatusID(%q): unexpected error: %v", tc.in, err)
                }
                if got != tc.want {
                        t.Fatalf("NormalizeStatusID(%q): expected %q, got %q", tc.in, tc.want, got)
                }
        }
}

func TestValidateStatusID(t *testing.T) {
        o := model.Outline{StatusDefs: []model.OutlineStatusDef{
                {ID: "todo"},
                {ID: "doing"},
                {ID: "done"},
        }}
        cases := []struct {
                in   string
                want bool
        }{
                {"", true},
                {"todo", true},
                {"done", true},
                {"nope", false},
        }
        for _, tc := range cases {
                if got := ValidateStatusID(o, tc.in); got != tc.want {
                        t.Fatalf("ValidateStatusID(%q): expected %v, got %v", tc.in, tc.want, got)
                }
        }
}

func TestIsEndState(t *testing.T) {
        o := model.Outline{StatusDefs: []model.OutlineStatusDef{
                {ID: "done", IsEndState: true},
                {ID: "todo", IsEndState: false},
        }}
        if IsEndState(o, "todo") {
                t.Fatalf("expected todo not end-state")
        }
        if !IsEndState(o, "done") {
                t.Fatalf("expected done end-state")
        }
        if IsEndState(o, "") {
                t.Fatalf("expected empty not end-state")
        }
        // Legacy fallback.
        if !IsEndState(model.Outline{}, "done") {
                t.Fatalf("expected fallback done end-state")
        }
}
