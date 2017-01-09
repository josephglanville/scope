package flynn

import (
	"net"

	"github.com/weaveworks/scope/probe"
	"github.com/weaveworks/scope/report"
)

// Exposed for testing
var (
	JobMetadataTemplates = report.MetadataTemplates{
		JobID:        {ID: JobID, Label: "ID", From: report.FromLatest, Truncate: 12, Priority: 1},
		JobArgs:      {ID: JobArgs, Label: "Args", From: report.FromLatest, Priority: 2},
		JobStatus:    {ID: JobStatus, Label: "Status", From: report.FromLatest, Priority: 3},
		JobUptime:    {ID: JobUptime, Label: "Uptime", From: report.FromLatest, Priority: 4},
		JobIPs:       {ID: JobIPs, Label: "IPs", From: report.FromSets, Priority: 5},
		JobCreated:   {ID: JobCreated, Label: "Created", From: report.FromLatest, Datatype: "datetime", Priority: 6},
		JobAppID:     {ID: JobAppID, Label: "AppID", From: report.FromLatest, Priority: 7},
		JobReleaseID: {ID: JobReleaseID, Label: "ReleaseID", From: report.FromLatest, Priority: 8},
		JobType:      {ID: JobType, Label: "Type", From: report.FromLatest, Priority: 9},
	}
)

// Reporter generate Reports containing Job topologies
type Reporter struct {
	host    FlynnHost
	hostID  string
	probeID string
	probe   *probe.Probe
}

// NewReporter makes a new Reporter
func NewReporter(host FlynnHost, hostID string, probeID string, probe *probe.Probe) *Reporter {
	reporter := &Reporter{
		host:    host,
		hostID:  hostID,
		probeID: probeID,
		probe:   probe,
	}
	// watch flynn job updates and generate shortcut report when job changes state
	host.WatchJobUpdates(reporter.JobUpdated)
	return reporter
}

// Name of this reporter, for metrics gathering
func (Reporter) Name() string { return "Flynn" }

// JobUpdated should be called whenever a job is updated.
func (r *Reporter) JobUpdated(n report.Node) {
	// Publish a 'short cut' report for just this job
	rpt := report.MakeReport()
	rpt.Shortcut = true
	rpt.FlynnJob.AddNode(n)
	r.probe.Publish(rpt)
}

// Report generates a Report containing Container and ContainerImage topologies
func (r *Reporter) Report() (report.Report, error) {
	localAddrs, err := report.LocalAddresses()
	if err != nil {
		return report.MakeReport(), nil
	}

	result := report.MakeReport()
	result.FlynnJob = result.FlynnJob.Merge(r.jobTopology(localAddrs))
	return result, nil
}

func (r *Reporter) jobTopology(localAddrs []net.IP) report.Topology {
	result := report.MakeTopology().
		WithMetadataTemplates(JobMetadataTemplates)

	metadata := map[string]string{report.ControlProbeID: r.probeID}
	nodes := []report.Node{}
	r.host.WalkJobs(func(j Job) {
		nodes = append(nodes, j.GetNode().WithLatests(metadata))
	})

	for _, node := range nodes {
		result.AddNode(node)
	}

	// TODO(jpg) deal with network scoping stuff
	// jobs that use host networking need to have local scoped addrs like docker
	// non-host networking jobs however have globally unique IPs like k8s
	return result
}
