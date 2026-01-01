package web

import (
        "crypto/hmac"
        "crypto/rand"
        "crypto/sha256"
        "encoding/base64"
        "encoding/json"
        "errors"
        "fmt"
        "os"
        "path/filepath"
        "strings"
        "time"
)

type signedPayload struct {
        Exp int64  `json:"exp"`
        Sub string `json:"sub"`           // actorId or email
        Typ string `json:"typ,omitempty"` // "session"|"magic"
        N   string `json:"n,omitempty"`   // nonce
}

func workspaceRootFromDir(dir string) string {
        dir = filepath.Clean(strings.TrimSpace(dir))
        if filepath.Base(dir) == ".clarity" {
                return filepath.Dir(dir)
        }
        return dir
}

func secretKeyPath(workspaceDir string) string {
        root := workspaceRootFromDir(workspaceDir)
        return filepath.Join(root, ".clarity", "web", "secret.key")
}

func loadOrInitSecretKey(workspaceDir string) ([]byte, error) {
        path := secretKeyPath(workspaceDir)
        if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
                return []byte(strings.TrimSpace(string(b))), nil
        }

        if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
                return nil, err
        }
        raw := make([]byte, 32)
        if _, err := rand.Read(raw); err != nil {
                return nil, err
        }
        enc := base64.RawURLEncoding.EncodeToString(raw)
        if err := os.WriteFile(path, []byte(enc+"\n"), 0o600); err != nil {
                return nil, err
        }
        return []byte(enc), nil
}

func signToken(secret []byte, payload signedPayload) (string, error) {
        b, err := json.Marshal(payload)
        if err != nil {
                return "", err
        }
        p := base64.RawURLEncoding.EncodeToString(b)
        mac := hmac.New(sha256.New, secret)
        _, _ = mac.Write([]byte(p))
        sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
        return p + "." + sig, nil
}

func verifyToken(secret []byte, token string) (signedPayload, error) {
        token = strings.TrimSpace(token)
        parts := strings.Split(token, ".")
        if len(parts) != 2 {
                return signedPayload{}, errors.New("invalid token format")
        }
        p, sig := parts[0], parts[1]

        mac := hmac.New(sha256.New, secret)
        _, _ = mac.Write([]byte(p))
        want := mac.Sum(nil)
        got, err := base64.RawURLEncoding.DecodeString(sig)
        if err != nil {
                return signedPayload{}, errors.New("invalid token signature")
        }
        if !hmac.Equal(want, got) {
                return signedPayload{}, errors.New("invalid token signature")
        }

        raw, err := base64.RawURLEncoding.DecodeString(p)
        if err != nil {
                return signedPayload{}, errors.New("invalid token payload")
        }
        var sp signedPayload
        if err := json.Unmarshal(raw, &sp); err != nil {
                return signedPayload{}, errors.New("invalid token payload")
        }
        if sp.Exp == 0 {
                return signedPayload{}, errors.New("token missing exp")
        }
        if time.Now().Unix() > sp.Exp {
                return signedPayload{}, errors.New("token expired")
        }
        if strings.TrimSpace(sp.Sub) == "" {
                return signedPayload{}, errors.New("token missing sub")
        }
        return sp, nil
}

func newNonce() (string, error) {
        b := make([]byte, 16)
        if _, err := rand.Read(b); err != nil {
                return "", err
        }
        return base64.RawURLEncoding.EncodeToString(b), nil
}

func newMagicToken(secret []byte, email string, ttl time.Duration) (string, error) {
        email = strings.ToLower(strings.TrimSpace(email))
        if email == "" {
                return "", errors.New("missing email")
        }
        n, err := newNonce()
        if err != nil {
                return "", err
        }
        return signToken(secret, signedPayload{
                Typ: "magic",
                Sub: email,
                N:   n,
                Exp: time.Now().Add(ttl).Unix(),
        })
}

func newSessionToken(secret []byte, actorID string, ttl time.Duration) (string, error) {
        actorID = strings.TrimSpace(actorID)
        if actorID == "" {
                return "", errors.New("missing actor")
        }
        n, err := newNonce()
        if err != nil {
                return "", err
        }
        return signToken(secret, signedPayload{
                Typ: "session",
                Sub: actorID,
                N:   n,
                Exp: time.Now().Add(ttl).Unix(),
        })
}

func writeOutboxEmail(workspaceDir, to, subject, body string) error {
        root := workspaceRootFromDir(workspaceDir)
        outDir := filepath.Join(root, ".clarity", "web", "outbox")
        if err := os.MkdirAll(outDir, 0o755); err != nil {
                return err
        }
        ts := time.Now().UTC().Format("20060102T150405Z")
        safeTo := strings.NewReplacer("@", "_at_", "/", "_").Replace(strings.ToLower(strings.TrimSpace(to)))
        name := fmt.Sprintf("%s_%s.txt", ts, safeTo)
        msg := fmt.Sprintf("TO: %s\nSUBJECT: %s\n\n%s\n", strings.TrimSpace(to), strings.TrimSpace(subject), strings.TrimSpace(body))
        return os.WriteFile(filepath.Join(outDir, name), []byte(msg), 0o600)
}
