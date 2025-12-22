package main

import (
        "reflect"
        "testing"
)

func TestRewriteDirectItemLookupArgs(t *testing.T) {
        t.Parallel()

        tests := []struct {
                name string
                in   []string
                want []string
        }{
                {
                        name: "no args",
                        in:   []string{"clarity"},
                        want: []string{"clarity"},
                },
                {
                        name: "direct item id first token",
                        in:   []string{"clarity", "item-abc123"},
                        want: []string{"clarity", "items", "show", "item-abc123"},
                },
                {
                        name: "direct item id after value flag",
                        in:   []string{"clarity", "--dir", "./tmp-test-ws", "item-abc123"},
                        want: []string{"clarity", "--dir", "./tmp-test-ws", "items", "show", "item-abc123"},
                },
                {
                        name: "direct item id after equals flag",
                        in:   []string{"clarity", "--dir=./tmp-test-ws", "item-abc123"},
                        want: []string{"clarity", "--dir=./tmp-test-ws", "items", "show", "item-abc123"},
                },
                {
                        name: "direct item id after bool flag",
                        in:   []string{"clarity", "--pretty", "item-abc123"},
                        want: []string{"clarity", "--pretty", "items", "show", "item-abc123"},
                },
                {
                        name: "direct item id after double dash",
                        in:   []string{"clarity", "--dir", "./tmp-test-ws", "--", "item-abc123"},
                        want: []string{"clarity", "--dir", "./tmp-test-ws", "--", "items", "show", "item-abc123"},
                },
                {
                        name: "normal subcommand not rewritten",
                        in:   []string{"clarity", "items", "show", "item-abc123"},
                        want: []string{"clarity", "items", "show", "item-abc123"},
                },
                {
                        name: "unknown command not rewritten",
                        in:   []string{"clarity", "wat"},
                        want: []string{"clarity", "wat"},
                },
        }

        for _, tt := range tests {
                tt := tt
                t.Run(tt.name, func(t *testing.T) {
                        t.Parallel()
                        got := rewriteDirectItemLookupArgs(tt.in)
                        if !reflect.DeepEqual(got, tt.want) {
                                t.Fatalf("rewriteDirectItemLookupArgs:\n got: %#v\nwant: %#v", got, tt.want)
                        }
                })
        }
}
