package store

import (
        "crypto/rand"
        "math/big"
        "strings"
)

// newRandomID returns prefix-<suffix> where suffix is base36 (lowercase).
// The suffix length is intentionally short for better ergonomics in a terminal UI.
func newRandomID(prefix string) (string, error) {
        return newRandomIDWithLen(prefix, idSuffixLen(prefix))
}

func newRandomIDWithLen(prefix string, suffixLen int) (string, error) {
        if suffixLen <= 0 {
                suffixLen = 8
        }

        // Base36 gives 36^3 = 46,656 variants for 3-char suffixes, which is plenty for a single workspace.
        // Use crypto/rand + rejection sampling via math/big for a uniform distribution.
        max := new(big.Int).Exp(big.NewInt(36), big.NewInt(int64(suffixLen)), nil)
        n, err := rand.Int(rand.Reader, max)
        if err != nil {
                return "", err
        }
        suffix := strings.ToLower(n.Text(36))
        if len(suffix) < suffixLen {
                suffix = strings.Repeat("0", suffixLen-len(suffix)) + suffix
        }
        return prefix + "-" + suffix, nil
}

func idSuffixLen(prefix string) int {
        switch prefix {
        case "item":
                // Items are referenced constantly; keep these extra short.
                return 3
        case "act", "proj", "out":
                // These are user-facing too (clipboard, scriptable commands).
                return 3
        default:
                return 8
        }
}

func idExists(db *DB, id string) bool {
        for _, a := range db.Actors {
                if a.ID == id {
                        return true
                }
        }
        for _, p := range db.Projects {
                if p.ID == id {
                        return true
                }
        }
        for _, o := range db.Outlines {
                if o.ID == id {
                        return true
                }
        }
        for _, it := range db.Items {
                if it.ID == id {
                        return true
                }
        }
        for _, d := range db.Deps {
                if d.ID == id {
                        return true
                }
        }
        for _, c := range db.Comments {
                if c.ID == id {
                        return true
                }
        }
        for _, w := range db.Worklog {
                if w.ID == id {
                        return true
                }
        }
        return false
}
