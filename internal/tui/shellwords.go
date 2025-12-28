package tui

import "unicode"

// splitShellWords splits a shell-like command string into argv, handling basic quoting.
// It supports single quotes, double quotes, and backslash escaping (outside single quotes).
func splitShellWords(s string) []string {
        var out []string
        var cur []rune
        inSingle := false
        inDouble := false
        escaped := false

        flush := func() {
                if len(cur) == 0 {
                        return
                }
                out = append(out, string(cur))
                cur = cur[:0]
        }

        for _, r := range []rune(s) {
                if escaped {
                        cur = append(cur, r)
                        escaped = false
                        continue
                }

                if r == '\\' && !inSingle {
                        escaped = true
                        continue
                }

                if r == '\'' && !inDouble {
                        inSingle = !inSingle
                        continue
                }

                if r == '"' && !inSingle {
                        inDouble = !inDouble
                        continue
                }

                if !inSingle && !inDouble && unicode.IsSpace(r) {
                        flush()
                        continue
                }

                cur = append(cur, r)
        }

        flush()
        return out
}
