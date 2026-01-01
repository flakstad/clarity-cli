package gitrepo

import (
	"context"
	"strings"
)

func PullRebase(ctx context.Context, dir string) error {
	_, err := runGit(ctx, dir, "pull", "--rebase")
	return err
}

func Push(ctx context.Context, dir string) error {
	_, err := runGit(ctx, dir, "push")
	return err
}

func IsNonFastForwardPushErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"non-fast-forward",
		"fetch first",
		"rejected",
		"updates were rejected",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}
