package publish

import (
        "errors"
        "os"
        "path/filepath"
        "strings"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

type WriteOptions struct {
        IncludeArchived bool
        IncludeWorklog  bool
        Overwrite       bool
        ActorID         string
}

type WriteResult struct {
        Written []string `json:"written"`
}

func WriteItem(db *store.DB, itemID string, toDir string, opt WriteOptions) (WriteResult, error) {
        if db == nil {
                return WriteResult{}, errors.New("missing db")
        }
        itemID = strings.TrimSpace(itemID)
        if itemID == "" {
                return WriteResult{}, errors.New("missing itemID")
        }
        toDir = strings.TrimSpace(toDir)
        if toDir == "" {
                return WriteResult{}, errors.New("missing --to")
        }
        toDir = filepath.Clean(toDir)

        md, err := RenderItemMarkdown(db, itemID, RenderOptions{
                IncludeArchived: opt.IncludeArchived,
                IncludeWorklog:  opt.IncludeWorklog,
                ActorID:         opt.ActorID,
        })
        if err != nil {
                return WriteResult{}, err
        }

        outDir := filepath.Join(toDir, "items")
        if err := os.MkdirAll(outDir, 0o755); err != nil {
                return WriteResult{}, err
        }
        outPath := filepath.Join(outDir, itemID+".md")
        if err := writeFile(outPath, []byte(md), opt.Overwrite); err != nil {
                return WriteResult{}, err
        }
        return WriteResult{Written: []string{outPath}}, nil
}

func WriteOutline(db *store.DB, outlineID string, toDir string, opt WriteOptions) (WriteResult, error) {
        if db == nil {
                return WriteResult{}, errors.New("missing db")
        }
        outlineID = strings.TrimSpace(outlineID)
        if outlineID == "" {
                return WriteResult{}, errors.New("missing outlineID")
        }
        toDir = strings.TrimSpace(toDir)
        if toDir == "" {
                return WriteResult{}, errors.New("missing --to")
        }
        toDir = filepath.Clean(toDir)

        // Collect outline items.
        all := make([]*model.Item, 0)
        for i := range db.Items {
                it := &db.Items[i]
                if strings.TrimSpace(it.OutlineID) != outlineID {
                        continue
                }
                all = append(all, it)
        }

        outlineDir := filepath.Join(toDir, "outlines", outlineID)
        itemsDir := filepath.Join(outlineDir, "items")
        if err := os.MkdirAll(itemsDir, 0o755); err != nil {
                return WriteResult{}, err
        }

        // Write outline index.
        indexMD, err := RenderOutlineIndexMarkdown(db, outlineID, all, RenderOptions{
                IncludeArchived: opt.IncludeArchived,
                IncludeWorklog:  opt.IncludeWorklog,
                ActorID:         opt.ActorID,
        })
        if err != nil {
                return WriteResult{}, err
        }
        indexPath := filepath.Join(outlineDir, "index.md")
        if err := writeFile(indexPath, []byte(indexMD), opt.Overwrite); err != nil {
                return WriteResult{}, err
        }

        // Write item pages (best-effort: stop on first error).
        written := []string{indexPath}
        for _, it := range all {
                if it == nil {
                        continue
                }
                if it.Archived && !opt.IncludeArchived {
                        continue
                }
                md, err := RenderItemMarkdown(db, it.ID, RenderOptions{
                        IncludeArchived: opt.IncludeArchived,
                        IncludeWorklog:  opt.IncludeWorklog,
                        ActorID:         opt.ActorID,
                })
                if err != nil {
                        return WriteResult{}, err
                }
                p := filepath.Join(itemsDir, it.ID+".md")
                if err := writeFile(p, []byte(md), opt.Overwrite); err != nil {
                        return WriteResult{}, err
                }
                written = append(written, p)
        }

        return WriteResult{Written: written}, nil
}

func writeFile(path string, b []byte, overwrite bool) error {
        if !overwrite {
                if _, err := os.Stat(path); err == nil {
                        return errors.New("file exists (use --overwrite): " + path)
                }
        }
        return os.WriteFile(path, b, 0o644)
}
