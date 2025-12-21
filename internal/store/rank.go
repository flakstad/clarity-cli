package store

import (
        "errors"
        "strings"
)

const rankAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

func rankDigit(c byte) (int, bool) {
        switch {
        case c >= '0' && c <= '9':
                return int(c - '0'), true
        case c >= 'a' && c <= 'z':
                return 10 + int(c-'a'), true
        default:
                return 0, false
        }
}

func rankChar(d int) byte {
        if d < 0 {
                d = 0
        }
        if d > 35 {
                d = 35
        }
        return rankAlphabet[d]
}

// RankBetween returns a lexicographic rank strictly between a and b.
// a may be empty (no lower bound) and b may be empty (no upper bound).
//
// Ranks are lowercase base36-like strings. The ordering is purely lexicographic.
// The algorithm is a simple fractional-indexing midpoint.
func RankBetween(a, b string) (string, error) {
        a = strings.ToLower(strings.TrimSpace(a))
        b = strings.ToLower(strings.TrimSpace(b))

        if a != "" && b != "" && !(a < b) {
                return "", errors.New("RankBetween requires a < b")
        }

        const min = 0
        const max = 35

        prefix := make([]byte, 0, 8)
        for i := 0; i < 256; i++ {
                da := min
                db := max
                if i < len(a) {
                        if v, ok := rankDigit(a[i]); ok {
                                da = v
                        } else {
                                return "", errors.New("invalid rank character in a")
                        }
                }
                if i < len(b) {
                        if v, ok := rankDigit(b[i]); ok {
                                db = v
                        } else {
                                return "", errors.New("invalid rank character in b")
                        }
                }

                if da == db {
                        prefix = append(prefix, rankChar(da))
                        continue
                }

                if db-da > 1 {
                        mid := da + (db-da)/2
                        prefix = append(prefix, rankChar(mid))
                        return string(prefix), nil
                }

                // Adjacent digits: extend a to guarantee a < result < b.
                // Since b differs at this position, any extension of a is still < b.
                return a + "0", nil
        }
        return "", errors.New("unable to compute rank between")
}

func RankAfter(a string) (string, error)  { return RankBetween(a, "") }
func RankBefore(b string) (string, error) { return RankBetween("", b) }
func RankInitial() (string, error)        { return RankBetween("", "") }

// RankBetweenUnique returns a rank between lower and upper that is not already present in existing.
//
// existing keys should be normalized (lowercase + trimmed). This function will also normalize the
// generated rank before checking existence.
//
// This is used to enforce "from now on, newly assigned ranks are unique" without rewriting other
// items in the sibling set.
func RankBetweenUnique(existing map[string]bool, lower, upper string) (string, error) {
        if existing == nil {
                existing = map[string]bool{}
        }
        lower = strings.ToLower(strings.TrimSpace(lower))
        upper = strings.ToLower(strings.TrimSpace(upper))

        // Try repeatedly tightening the lower bound. RankBetween guarantees strictly between bounds
        // when both are non-empty, so each iteration produces a different value.
        curLower := lower
        for i := 0; i < 256; i++ {
                r, err := RankBetween(curLower, upper)
                if err != nil {
                        return "", err
                }
                rn := strings.ToLower(strings.TrimSpace(r))
                if rn == "" {
                        // Extremely defensive: should never happen, but avoid infinite loops.
                        return "", errors.New("generated empty rank")
                }
                if !existing[rn] {
                        return rn, nil
                }
                // Collision: move the lower bound up and try again.
                curLower = rn
        }
        return "", errors.New("unable to find unique rank")
}
