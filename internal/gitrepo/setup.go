package gitrepo

import (
        "context"
        "errors"
        "strings"
)

func Init(ctx context.Context, dir string) error {
        _, err := runGit(ctx, dir, "init")
        return err
}

func SetRemoteURL(ctx context.Context, dir, remoteName, remoteURL string) error {
        remoteName = strings.TrimSpace(remoteName)
        remoteURL = strings.TrimSpace(remoteURL)
        if remoteName == "" {
                remoteName = "origin"
        }
        if remoteURL == "" {
                return errors.New("empty remote url")
        }

        // If remote exists, update; otherwise add.
        if _, err := runGit(ctx, dir, "remote", "get-url", remoteName); err == nil {
                _, err := runGit(ctx, dir, "remote", "set-url", remoteName, remoteURL)
                return err
        }
        _, err := runGit(ctx, dir, "remote", "add", remoteName, remoteURL)
        return err
}

func PushSetUpstream(ctx context.Context, dir, remoteName, branch string) error {
        remoteName = strings.TrimSpace(remoteName)
        branch = strings.TrimSpace(branch)
        if remoteName == "" {
                remoteName = "origin"
        }
        if branch == "" {
                branch = "HEAD"
        }
        _, err := runGit(ctx, dir, "push", "-u", remoteName, branch)
        return err
}
