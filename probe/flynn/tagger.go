package flynn

import (
	"strconv"

	"github.com/weaveworks/scope/probe/process"
	"github.com/weaveworks/scope/report"
)

var (
	NewProcessTreeStub = process.NewTree
)

// Tagger is a tagger that tags Docker container information to process
// nodes that have a PID.
type Tagger struct {
	host       FlynnHost
	procWalker process.Walker
}

// NewTagger returns a usable Tagger.
func NewTagger(host FlynnHost, procWalker process.Walker) *Tagger {
	return &Tagger{
		host:       host,
		procWalker: procWalker,
	}
}

// Name of this tagger, for metrics gathering
func (Tagger) Name() string { return "Flynn" }

// Tag implements Tagger.
func (t *Tagger) Tag(r report.Report) (report.Report, error) {
	tree, err := NewProcessTreeStub(t.procWalker)
	if err != nil {
		return report.MakeReport(), err
	}
	t.tag(tree, &r.Process)
	return r, nil
}

func (t *Tagger) tag(tree process.Tree, topology *report.Topology) {
	for nodeID, node := range topology.Nodes {
		pidStr, ok := node.Latest.Lookup(process.PID)
		if !ok {
			continue
		}

		pid, err := strconv.ParseUint(pidStr, 10, 64)
		if err != nil {
			continue
		}

		var (
			c         Job
			candidate = int(pid)
		)

		t.host.LockedPIDLookup(func(lookup func(int) Job) {
			for {
				c = lookup(candidate)
				if c != nil {
					break
				}

				candidate, err = tree.GetParent(candidate)
				if err != nil {
					break
				}
			}
		})

		if c == nil || JobIsStopped(c) || c.PID() == 1 {
			continue
		}

		node := report.MakeNodeWith(nodeID, map[string]string{
			JobID: c.ID(),
		}).WithParents(report.EmptySets.
			Add(report.FlynnJob, report.MakeStringSet(report.MakeFlynnJobNodeID(c.ID()))),
		)

		topology.AddNode(node)
	}
}
