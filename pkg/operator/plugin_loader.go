package operator

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PluginManager struct {
	cacheDir string
}

func NewPluginManager(cacheDir string) *PluginManager {
	return &PluginManager{
		cacheDir: cacheDir,
	}
}

func (pm *PluginManager) LoadPlugin(
	ctx context.Context,
	source *manifest.PluginSource,
) (*PluginOperator, error) {
	var scriptPath string
	var err error

	switch source.Type {
	case "file":
		if source.File == nil {
			return nil, fmt.Errorf("file source config is required")
		}
		scriptPath = source.File.Path

	case "git":
		if source.Git == nil {
			return nil, fmt.Errorf("git source config is required")
		}
		scriptPath, _, err = pm.fetchFromGit(ctx, source.Git)
		if err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported plugin source type: %s", source.Type)
	}

	op, err := NewPluginOperator(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load plugin operator: %w", err)
	}

	return op, nil
}

func (pm *PluginManager) fetchFromGit(
	ctx context.Context,
	gitSource *manifest.GitPluginSource,
) (string, *manifest.PluginStatus, error) {
	status := &manifest.PluginStatus{}

	hash := sha256.Sum256([]byte(gitSource.URL))
	repoDir := filepath.Join(pm.cacheDir, fmt.Sprintf("%x", hash[:8]))

	var repo *git.Repository
	var err error

	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		cloneOpts := &git.CloneOptions{
			URL:      gitSource.URL,
			Progress: os.Stdout,
		}

		if gitSource.Auth != nil {
			// TODO: Load credentials from secret
			cloneOpts.Auth = &http.BasicAuth{
				Username: "username", // from secret
				Password: "token",    // from secret
			}
		}

		repo, err = git.PlainCloneContext(ctx, repoDir, false, cloneOpts)
		if err != nil {
			return "", status, fmt.Errorf("failed to clone repository: %w", err)
		}
	} else {
		repo, err = git.PlainOpen(repoDir)
		if err != nil {
			return "", status, fmt.Errorf("failed to open repository: %w", err)
		}

		err = repo.FetchContext(ctx, &git.FetchOptions{
			RemoteName: "origin",
		})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return "", status, fmt.Errorf("failed to fetch updates: %w", err)
		}
	}

	w, err := repo.Worktree()
	if err != nil {
		return "", status, fmt.Errorf("failed to get worktree: %w", err)
	}

	checkoutOpts := &git.CheckoutOptions{}

	if gitSource.Ref != "" {
		refName := plumbing.NewBranchReferenceName(gitSource.Ref)
		checkoutOpts.Branch = refName

		err = w.Checkout(checkoutOpts)
		if err != nil {
			refName = plumbing.NewTagReferenceName(gitSource.Ref)
			checkoutOpts.Branch = refName

			err = w.Checkout(checkoutOpts)
			if err != nil {
				hash := plumbing.NewHash(gitSource.Ref)
				checkoutOpts.Hash = hash
				checkoutOpts.Branch = ""

				err = w.Checkout(checkoutOpts)
				if err != nil {
					return "", status, fmt.Errorf(
						"failed to checkout ref %s: %w",
						gitSource.Ref,
						err,
					)
				}
			}
		}
	}

	head, err := repo.Head()
	if err != nil {
		return "", status, fmt.Errorf("failed to get HEAD: %w", err)
	}
	status.GitCommit = head.Hash().String()
	status.LastUpdated = metav1.Now()

	scriptPath := filepath.Join(repoDir, gitSource.Path)

	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return "", status, fmt.Errorf(
			"script not found at path: %s",
			gitSource.Path,
		)
	}

	return scriptPath, status, nil
}
