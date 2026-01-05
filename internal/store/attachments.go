package store

import (
        "crypto/sha256"
        "encoding/hex"
        "errors"
        "fmt"
        "io"
        "mime"
        "os"
        "path/filepath"
        "strings"
        "time"

        "clarity-cli/internal/model"
)

const DefaultAttachmentMaxBytes int64 = 50 * 1024 * 1024 // 50MB

func (s Store) attachmentsDir() string {
        return filepath.Join(s.workspaceRoot(), "resources", "attachments")
}

func (s Store) attachmentFilePath(a model.Attachment) string {
        return filepath.Join(s.workspaceRoot(), filepath.FromSlash(strings.TrimSpace(a.Path)))
}

func (s Store) AttachmentAbsPath(a model.Attachment) string {
        return s.attachmentFilePath(a)
}

func normalizeAttachmentEntityKind(kind string) (string, error) {
        switch strings.ToLower(strings.TrimSpace(kind)) {
        case "item":
                return "item", nil
        case "comment":
                return "comment", nil
        default:
                return "", fmt.Errorf("invalid attachment entity kind: %q (expected item|comment)", kind)
        }
}

func guessMimeType(filename string) string {
        ext := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
        if ext == "" {
                return ""
        }
        return mime.TypeByExtension(ext)
}

func (s Store) AddAttachment(db *DB, actorID string, entityKind string, entityID string, srcPath string, title string, alt string, maxBytes int64) (model.Attachment, error) {
        if db == nil {
                return model.Attachment{}, errors.New("nil db")
        }
        actorID = strings.TrimSpace(actorID)
        if actorID == "" {
                return model.Attachment{}, errors.New("missing actor id")
        }
        kind, err := normalizeAttachmentEntityKind(entityKind)
        if err != nil {
                return model.Attachment{}, err
        }
        entityID = strings.TrimSpace(entityID)
        if entityID == "" {
                return model.Attachment{}, errors.New("missing entity id")
        }
        srcPath = filepath.Clean(strings.TrimSpace(srcPath))
        if srcPath == "" {
                return model.Attachment{}, errors.New("missing source path")
        }
        st, err := os.Stat(srcPath)
        if err != nil {
                return model.Attachment{}, err
        }
        if st.IsDir() {
                return model.Attachment{}, errors.New("attachments: source path is a directory")
        }
        if maxBytes <= 0 {
                maxBytes = DefaultAttachmentMaxBytes
        }
        if st.Size() > maxBytes {
                return model.Attachment{}, fmt.Errorf("attachments: file too large (%d bytes > %d bytes)", st.Size(), maxBytes)
        }

        orig := filepath.Base(srcPath)
        if strings.TrimSpace(orig) == "" || orig == "." || orig == string(filepath.Separator) {
                orig = "attachment"
        }

        now := time.Now().UTC()
        id := s.NextID(db, "att")
        destDir := filepath.Join(s.attachmentsDir(), id)
        if err := os.MkdirAll(destDir, 0o755); err != nil {
                return model.Attachment{}, err
        }

        // Keep a stable on-disk name for now; metadata stores original name separately.
        destName := orig
        destPath := filepath.Join(destDir, destName)

        in, err := os.Open(srcPath)
        if err != nil {
                return model.Attachment{}, err
        }
        defer in.Close()

        out, err := os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
        if err != nil {
                return model.Attachment{}, err
        }
        defer func() { _ = out.Close() }()

        h := sha256.New()
        w := io.MultiWriter(out, h)
        n, err := io.Copy(w, io.LimitReader(in, maxBytes+1))
        if err != nil {
                return model.Attachment{}, err
        }
        if n > maxBytes {
                return model.Attachment{}, fmt.Errorf("attachments: file too large (%d bytes > %d bytes)", n, maxBytes)
        }

        sum := hex.EncodeToString(h.Sum(nil))

        rel := filepath.ToSlash(filepath.Join("resources", "attachments", id, destName))
        a := model.Attachment{
                ID:           id,
                EntityKind:   kind,
                EntityID:     entityID,
                Title:        strings.TrimSpace(title),
                Alt:          strings.TrimSpace(alt),
                OriginalName: orig,
                SizeBytes:    n,
                MimeType:     guessMimeType(orig),
                Sha256Hex:    sum,
                Path:         rel,
                CreatedBy:    actorID,
                CreatedAt:    now,
                UpdatedAt:    now,
        }

        db.Attachments = append(db.Attachments, a)
        // Indexes are now stale.
        db.idxBuilt = false
        return a, nil
}

func (s Store) UpdateAttachmentMetadata(db *DB, actorID string, attachmentID string, title string, alt string) (model.Attachment, error) {
        if db == nil {
                return model.Attachment{}, errors.New("nil db")
        }
        actorID = strings.TrimSpace(actorID)
        if actorID == "" {
                return model.Attachment{}, errors.New("missing actor id")
        }
        attachmentID = strings.TrimSpace(attachmentID)
        if attachmentID == "" {
                return model.Attachment{}, errors.New("missing attachment id")
        }

        now := time.Now().UTC()
        for i := range db.Attachments {
                if strings.TrimSpace(db.Attachments[i].ID) != attachmentID {
                        continue
                }
                db.Attachments[i].Title = strings.TrimSpace(title)
                db.Attachments[i].Alt = strings.TrimSpace(alt)
                db.Attachments[i].UpdatedAt = now
                // Do not touch CreatedAt/CreatedBy/Path/etc.
                db.idxBuilt = false
                return db.Attachments[i], nil
        }
        return model.Attachment{}, fmt.Errorf("attachment not found: %s", attachmentID)
}
