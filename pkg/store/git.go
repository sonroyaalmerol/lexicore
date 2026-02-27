package store

import (
	"context"
	"os"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

type GitStore struct {
	repoURL  string
	branch   string
	localDir string
	username string
	password string
	interval time.Duration

	inner *FileStore
}

func NewGitStore(
	repoURL, branch, localDir, username, password string,
	pollInterval time.Duration,
) (*GitStore, error) {
	g := &GitStore{
		repoURL:  repoURL,
		branch:   branch,
		localDir: localDir,
		username: username,
		password: password,
		interval: pollInterval,
		inner:    NewFileStore(localDir),
	}

	if err := g.clone(); err != nil {
		return nil, err
	}

	return g, nil
}

func (g *GitStore) auth() *http.BasicAuth {
	if g.username == "" {
		return nil
	}
	return &http.BasicAuth{
		Username: g.username,
		Password: g.password,
	}
}

func (g *GitStore) clone() error {
	if _, err := os.Stat(g.localDir); err == nil {
		return g.pull()
	}

	_, err := gogit.PlainClone(g.localDir, false, &gogit.CloneOptions{
		URL:  g.repoURL,
		Auth: g.auth(),
	})
	return err
}

func (g *GitStore) pull() error {
	repo, err := gogit.PlainOpen(g.localDir)
	if err != nil {
		return err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	err = wt.Pull(&gogit.PullOptions{
		Auth: g.auth(),
	})
	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		return err
	}

	return nil
}

func (g *GitStore) GetIdentitySources(
	ctx context.Context,
) ([]*manifest.IdentitySource, error) {
	return g.inner.GetIdentitySources(ctx)
}

func (g *GitStore) GetSyncTargets(
	ctx context.Context,
) ([]*manifest.SyncTarget, error) {
	return g.inner.GetSyncTargets(ctx)
}

func (g *GitStore) Watch(ctx context.Context, onChange func()) error {
	go func() {
		ticker := time.NewTicker(g.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := g.pull(); err == nil {
					onChange()
				}
			}
		}
	}()

	return nil
}
