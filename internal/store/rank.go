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
