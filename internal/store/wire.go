package store

import (
        "encoding/json"
        "strings"
)

func loadWireDB(b []byte) (DB, map[string]int, error) {
        var db DB
        if err := json.Unmarshal(b, &db); err != nil {
                return DB{}, nil, err
        }

        legacyOrder := map[string]int{}

        // Extract order from the wire format if present (legacy).
        var raw map[string]json.RawMessage
        if err := json.Unmarshal(b, &raw); err == nil {
                itemsRaw := raw["items"]
                if isNullOrEmpty(itemsRaw) {
                        itemsRaw = raw["tasks"]
                }
                if !isNullOrEmpty(itemsRaw) {
                        var items []struct {
                                ID    string `json:"id"`
                                Order int    `json:"order,omitempty"`
                        }
                        if err := json.Unmarshal(itemsRaw, &items); err == nil {
                                for _, it := range items {
                                        if strings.TrimSpace(it.ID) == "" {
                                                continue
                                        }
                                        if it.Order != 0 {
                                                legacyOrder[it.ID] = it.Order
                                        }
                                }
                        }
                }
        }

        return db, legacyOrder, nil
}

func isNullOrEmpty(b []byte) bool {
        if len(b) == 0 {
                return true
        }
        s := strings.TrimSpace(string(b))
        return s == "" || s == "null"
}
