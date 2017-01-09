package render

import (
	"fmt"

	"github.com/weaveworks/scope/probe/flynn"
	"github.com/weaveworks/scope/report"
)

// FlynnJobRenderer is a Renderer which produces a renderable job
// graph by merging the process graph and the flynn_job topology.
// NB We only want processes in jobs _or_ processes with network connections
// but we need to be careful to ensure we only include each edge once, by only
// including the ProcessRenderer once.
var FlynnJobRenderer = MakeFilter(
	func(n report.Node) bool {
		// Drop deleted jobs
		state, ok := n.Latest.Lookup(flynn.JobStatus)
		return !ok || state == flynn.StatusRunning
	},
	MakeReduce(
		MakeMap(
			MapProcess2FlynnJob,
			ProcessRenderer,
		),

		// This mapper brings in short lived connections by joining with job IPs.
		// We need to be careful to ensure we only include each edge once.  Edges brought in
		// by the above renders will have a pid, so its enough to filter out any nodes with
		// pids.
		ShortLivedConnectionJoin(SelectFlynnJob, MapFlynnJob2IP),
		SelectFlynnJob,
	),
)

// MapFlynnJob2IP maps job nodes to their IP addresses (outputs
// multiple nodes).  This allows job to be joined directly with
// the endpoint topology.
func MapFlynnJob2IP(m report.Node) []string {
	_, ok := m.Latest.Lookup(flynn.JobID)
	if !ok {
		return nil
	}
	// if this job doesn't make connections, we can ignore it
	_, doesntMakeConnections := m.Latest.Lookup(report.DoesNotMakeConnections)
	// if this job belongs to the host's networking namespace
	// we cannot use its IP to attribute connections
	// (they could come from any other process on the host or DNAT-ed IPs)
	//hostNetwork, ok := m.Latest.Lookup(flynn.JobHostNetwork)
	if doesntMakeConnections { // || (ok && hostNetwork == "true") {
		fmt.Println("no connections")
		return nil
	}

	result := []string{}
	if addrs, ok := m.Sets.Lookup(flynn.JobIPs); ok {
		for _, addr := range addrs {
			id := report.MakeScopedEndpointNodeID("", addr, "")
			result = append(result, id)
		}
	}
	return result
}

// MapProcess2FlynnJob maps process Nodes to job
// Nodes.
//
// If this function is given a node without a flynn_job_id
// (including other pseudo nodes), it will produce an "Uncontained"
// pseudo node.
//
// Otherwise, this function will produce a node with the correct ID
// format for a job, but without any Major or Minor labels.
// It does not have enough info to do that, and the resulting graph
// must be merged with a flynn_job graph to get that info.
func MapProcess2FlynnJob(n report.Node, _ report.Networks) report.Nodes {
	// Propagate pseudo nodes
	if n.Topology == Pseudo {
		return report.Nodes{n.ID: n}
	}

	// Otherwise, if the process is not in a job, group it
	// into an per-host "Uncontained" node.  If for whatever reason
	// this node doesn't have a host id in their nodemetadata, it'll
	// all get grouped into a single uncontained node.
	var (
		id   string
		node report.Node
	)
	if jobID, ok := n.Latest.Lookup(flynn.JobID); ok {
		id = report.MakeFlynnJobNodeID(jobID)
		node = NewDerivedNode(id, n).WithTopology(report.FlynnJob)
	} else {
		id = MakePseudoNodeID(UncontainedID, report.ExtractHostID(n))
		node = NewDerivedPseudoNode(id, n)
		node = propagateLatest(report.HostNodeID, n, node)
		node = propagateLatest(IsConnected, n, node)
	}
	return report.Nodes{id: node}
}
