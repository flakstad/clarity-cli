package store

import (
        "crypto/rand"
        "encoding/base32"
        "strings"
)

// newRandomID returns prefix-<suffix> where suffix is 8 chars of base32 (lowercase, no padding).
// 8 chars base32 ~= 40 bits (~1 trillion) of space.
func newRandomID(prefix string) (string, error) {
        var b [5]byte // 40 bits -> 8 base32 chars
        if _, err := rand.Read(b[:]); err != nil {
                return "", err
        }
        enc := base32.StdEncoding.WithPadding(base32.NoPadding)
        suffix := strings.ToLower(enc.EncodeToString(b[:]))
        return prefix + "-" + suffix, nil
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
