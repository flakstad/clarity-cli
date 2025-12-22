package main

import (
        "os"
        "strings"

        "clarity-cli/internal/cli"
)

func isItemID(s string) bool {
        s = strings.TrimSpace(s)
        if !strings.HasPrefix(s, "item-") {
                return false
        }
        // Keep it permissive; IDs are generated but users may paste variants.
        return len(s) > len("item-")
}

func rewriteDirectItemLookupArgs(argv []string) []string {
        // Convenience: `clarity <item-id>` works like `clarity items show <item-id>`.
        //
        // Cobra treats the first non-flag token as a subcommand, so we rewrite argv before parsing.
        //
        // IMPORTANT: Users often pass persistent flags first (e.g. `clarity --dir ... <item-id>`),
        // so we must find the first positional token, not just argv[1].
        if len(argv) < 2 {
                return argv
        }

        // Minimal persistent-flag awareness. If we see flags we don't recognize, we skip them
        // (and do NOT try to skip their value) to avoid accidentally consuming the item-id.
        valueFlags := map[string]bool{
                "--dir":       true,
                "--workspace": true,
                "--actor":     true,
                "--format":    true,
        }
        boolFlags := map[string]bool{
                "--pretty": true,
        }

        for i := 1; i < len(argv); i++ {
                a := strings.TrimSpace(argv[i])
                if a == "" {
                        continue
                }
                if a == "--" {
                        // Stop flag parsing; next token (if any) is the first positional.
                        if i+1 < len(argv) && isItemID(argv[i+1]) {
                                out := make([]string, 0, len(argv)+2)
                                out = append(out, argv[:i+1]...)
                                out = append(out, "items", "show")
                                out = append(out, argv[i+1:]...)
                                return out
                        }
                        return argv
                }

                if strings.HasPrefix(a, "-") {
                        // --flag=value form
                        if strings.Contains(a, "=") {
                                continue
                        }
                        if boolFlags[a] {
                                continue
                        }
                        if valueFlags[a] {
                                i++ // skip value if present
                                continue
                        }
                        continue
                }

                // First positional token.
                if isItemID(a) {
                        out := make([]string, 0, len(argv)+2)
                        out = append(out, argv[:i]...)
                        out = append(out, "items", "show")
                        out = append(out, argv[i:]...)
                        return out
                }
                return argv
        }

        return argv
}

func main() {
        os.Args = rewriteDirectItemLookupArgs(os.Args)

        cmd := cli.NewRootCmd()
        if err := cmd.Execute(); err != nil {
                os.Exit(1)
        }
}
