package store

import (
	"context"
	"os"
	"path/filepath"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"github.com/fsnotify/fsnotify"
	"github.com/goccy/go-yaml"
)

type FileStore struct {
	dir    string
	parser *manifest.Parser
}

func NewFileStore(dir string) *FileStore {
	return &FileStore{
		dir:    dir,
		parser: manifest.NewParser(),
	}
}

func (f *FileStore) load(ctx context.Context) (
	[]*manifest.IdentitySource,
	[]*manifest.SyncTarget,
	error,
) {
	var sources []*manifest.IdentitySource
	var targets []*manifest.SyncTarget

	entries, err := os.ReadDir(f.dir)
	if err != nil {
		return nil, nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(f.dir, entry.Name()))
		if err != nil {
			continue
		}

		m, err := f.parser.Parse(data)
		if err != nil {
			continue
		}

		switch v := m.(type) {
		case *manifest.IdentitySource:
			sources = append(sources, v)
		case *manifest.SyncTarget:
			targets = append(targets, v)
		}
	}

	return sources, targets, nil
}

func (f *FileStore) GetIdentitySources(
	ctx context.Context,
) ([]*manifest.IdentitySource, error) {
	sources, _, err := f.load(ctx)
	return sources, err
}

func (f *FileStore) GetSyncTargets(
	ctx context.Context,
) ([]*manifest.SyncTarget, error) {
	_, targets, err := f.load(ctx)
	return targets, err
}

func (f *FileStore) Watch(ctx context.Context, onChange func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err := watcher.Add(f.dir); err != nil {
		watcher.Close()
		return err
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				ext := filepath.Ext(event.Name)
				if ext == ".yaml" || ext == ".yml" {
					onChange()
				}
			case <-watcher.Errors:
			}
		}
	}()

	return nil
}

func parseYAMLFile[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v T
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
