package tui

// stripANSIEscapes removes ANSI CSI escape sequences from a string.
// It is intentionally minimal: good enough for detecting "visually empty" lines
// produced by markdown renderers, without pulling in extra dependencies.
//
// This is used in `item_side_panel.go` to avoid counting ANSI-only spacer lines
// against available vertical space.
func stripANSIEscapes(s string) string {
        if s == "" {
                return ""
        }
        b := []byte(s)
        out := make([]byte, 0, len(b))
        for i := 0; i < len(b); i++ {
                if b[i] != 0x1b { // ESC
                        out = append(out, b[i])
                        continue
                }
                // CSI: ESC [
                if i+1 < len(b) && b[i+1] == '[' {
                        i += 2
                        // Consume until final byte (0x40-0x7E).
                        for i < len(b) {
                                c := b[i]
                                if c >= 0x40 && c <= 0x7E {
                                        break
                                }
                                i++
                        }
                        continue
                }
                // Unknown ESC sequence: drop just the ESC byte.
        }
        return string(out)
}
