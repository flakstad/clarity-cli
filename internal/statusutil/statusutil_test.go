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

func TestRequiresNote(t *testing.T) {
	o := model.Outline{StatusDefs: []model.OutlineStatusDef{
		{ID: "todo", RequiresNote: false},
		{ID: "blocked", RequiresNote: true},
	}}
	if RequiresNote(o, "") {
		t.Fatalf("expected empty to not require note")
	}
	if RequiresNote(o, "todo") {
		t.Fatalf("expected todo to not require note")
	}
	if !RequiresNote(o, "blocked") {
		t.Fatalf("expected blocked to require note")
	}
}

func TestCheckboxStatusIDs(t *testing.T) {
	o := model.Outline{StatusDefs: []model.OutlineStatusDef{
		{ID: "todo", IsEndState: false},
		{ID: "review", IsEndState: false},
		{ID: "done", IsEndState: true},
		{ID: "closed", IsEndState: true},
	}}
	if got := CheckboxUncheckedStatusID(o); got != "todo" {
		t.Fatalf("expected unchecked=todo; got %q", got)
	}
	if got := CheckboxCheckedStatusID(o); got != "done" {
		t.Fatalf("expected checked=done; got %q", got)
	}
	if !IsCheckboxChecked(o, "done") {
		t.Fatalf("expected done to be checked")
	}
	if !IsCheckboxChecked(o, "closed") {
		t.Fatalf("expected closed to be checked")
	}
	if IsCheckboxChecked(o, "todo") {
		t.Fatalf("expected todo to be unchecked")
	}

	// If there is no end-state, fall back to the last status id.
	o2 := model.Outline{StatusDefs: []model.OutlineStatusDef{
		{ID: "open"},
		{ID: "closed"},
	}}
	if got := CheckboxCheckedStatusID(o2); got != "closed" {
		t.Fatalf("expected checked=closed; got %q", got)
	}
	if got := CheckboxUncheckedStatusID(o2); got != "open" {
		t.Fatalf("expected unchecked=open; got %q", got)
	}
}
