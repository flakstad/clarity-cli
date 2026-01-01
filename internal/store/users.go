package store

import (
        "encoding/json"
        "errors"
        "os"
        "path/filepath"
        "sort"
        "strings"
)

// UsersFile is a workspace-committed mapping from human identities (email) to Clarity actors.
//
// Intended path: <workspaceRoot>/meta/users.json
//
// V1: only email → actorId is modeled. Future versions can add roles, scopes, and project-level access rules.
type UsersFile struct {
        Users []UserRef `json:"users"`
}

type UserRef struct {
        Email   string `json:"email"`
        ActorID string `json:"actorId"`
}

func UsersPath(workspaceDir string) string {
        workspaceDir = filepath.Clean(strings.TrimSpace(workspaceDir))
        if filepath.Base(workspaceDir) == ".clarity" {
                workspaceDir = filepath.Dir(workspaceDir)
        }
        return filepath.Join(workspaceDir, "meta", "users.json")
}

func LoadUsers(workspaceDir string) (UsersFile, bool, error) {
        path := UsersPath(workspaceDir)
        b, err := os.ReadFile(path)
        if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                        return UsersFile{}, false, nil
                }
                return UsersFile{}, false, err
        }

        var f UsersFile
        if err := json.Unmarshal(b, &f); err != nil {
                return UsersFile{}, true, err
        }
        if f.Users == nil {
                f.Users = []UserRef{}
        }

        // Normalize + validate.
        out := make([]UserRef, 0, len(f.Users))
        for _, u := range f.Users {
                email := strings.ToLower(strings.TrimSpace(u.Email))
                actorID := strings.TrimSpace(u.ActorID)
                if email == "" && actorID == "" {
                        continue
                }
                if email == "" {
                        return UsersFile{}, true, errors.New("meta/users.json: user missing email")
                }
                if actorID == "" {
                        return UsersFile{}, true, errors.New("meta/users.json: user missing actorId")
                }
                out = append(out, UserRef{Email: email, ActorID: actorID})
        }
        sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
        f.Users = out

        return f, true, nil
}

func (f UsersFile) ActorIDForEmail(email string) (string, bool) {
        email = strings.ToLower(strings.TrimSpace(email))
        if email == "" {
                return "", false
        }
        for _, u := range f.Users {
                if strings.ToLower(strings.TrimSpace(u.Email)) == email {
                        return strings.TrimSpace(u.ActorID), true
                }
        }
        return "", false
}
