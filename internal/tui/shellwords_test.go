package tui

import (
        "reflect"
        "testing"
)

func TestSplitShellWords(t *testing.T) {
        t.Parallel()

        tests := []struct {
                in   string
                want []string
        }{
                {"", nil},
                {"vim", []string{"vim"}},
                {"code --wait", []string{"code", "--wait"}},
                {"vim -u 'foo bar'", []string{"vim", "-u", "foo bar"}},
                {"vim -c \"set ft=markdown\"", []string{"vim", "-c", "set ft=markdown"}},
                {"vim\\ -u\\ foo", []string{"vim -u foo"}},
        }

        for _, tt := range tests {
                if got := splitShellWords(tt.in); !reflect.DeepEqual(got, tt.want) {
                        t.Fatalf("splitShellWords(%q)=%v, want %v", tt.in, got, tt.want)
                }
        }
}
