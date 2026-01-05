package gitrepo

import (
        "context"
        "strings"
)

// RemoteURL returns the configured fetch URL for a remote (e.g. origin).
func RemoteURL(ctx context.Context, dir, remoteName string) (string, error) {
        remoteName = strings.TrimSpace(remoteName)
        if remoteName == "" {
                remoteName = "origin"
        }
        out, err := git(ctx, dir, "remote", "get-url", remoteName)
        if err != nil {
                return "", err
        }
        return strings.TrimSpace(out), nil
}

type Remote struct {
        Name     string `json:"name"`
        FetchURL string `json:"fetchUrl,omitempty"`
        PushURL  string `json:"pushUrl,omitempty"`
}

func ListRemotes(ctx context.Context, dir string) ([]Remote, error) {
        out, err := git(ctx, dir, "remote")
        if err != nil {
                return nil, err
        }
        var remotes []Remote
        for _, line := range strings.Split(out, "\n") {
                name := strings.TrimSpace(strings.TrimRight(line, "\r"))
                if name == "" {
                        continue
                }
                r := Remote{Name: name}
                if fetchURL, err := git(ctx, dir, "remote", "get-url", name); err == nil {
                        r.FetchURL = strings.TrimSpace(fetchURL)
                }
                if pushURL, err := git(ctx, dir, "remote", "get-url", "--push", name); err == nil {
                        r.PushURL = strings.TrimSpace(pushURL)
                }
                remotes = append(remotes, r)
        }
        return remotes, nil
}
