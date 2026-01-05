package store

import (
        "errors"
        "io"
        "os"
        "path/filepath"
)

func CopyFile(src string, dest string) error {
        src = filepath.Clean(src)
        dest = filepath.Clean(dest)
        if src == "" || dest == "" {
                return errors.New("copy file: missing src/dest")
        }
        in, err := os.Open(src)
        if err != nil {
                return err
        }
        defer in.Close()

        if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
                return err
        }
        out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
        if err != nil {
                return err
        }
        defer func() { _ = out.Close() }()

        if _, err := io.Copy(out, in); err != nil {
                return err
        }
        return out.Close()
}
