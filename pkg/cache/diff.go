package cache

import "codeberg.org/lexicore/lexicore/pkg/source"

type Diff struct {
	IdentitiesToCreate     []*source.Identity
	IdentitiesToUpdate     []*source.Identity
	IdentitiesToDelete     []*source.Identity
	GroupsToCreate         []*source.Group
	GroupsToUpdate         []*source.Group
	GroupsToDelete         []*source.Group
	IdentitiesToReprocess  []*source.Identity
	HasChanges             bool
	TotalChanges           int
	SnapshotDelta          int64
	AffectedByGroupChanges bool
}

func (d *Diff) finalize() {
	d.TotalChanges = len(d.IdentitiesToCreate) +
		len(d.IdentitiesToUpdate) +
		len(d.IdentitiesToDelete) +
		len(d.GroupsToCreate) +
		len(d.GroupsToUpdate) +
		len(d.GroupsToDelete) +
		len(d.IdentitiesToReprocess)
	d.HasChanges = d.TotalChanges > 0
}
