package flynn

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flynn/flynn/host/types"
	"github.com/weaveworks/common/mtime"
	"github.com/weaveworks/scope/report"
)

const (
	JobID          = "flynn_job_id"
	JobArgs        = "flynn_job_args"
	JobStatus      = "flynn_job_status"
	JobUptime      = "flynn_job_uptime"
	JobIPs         = "flynn_job_ips"
	JobCreated     = "flynn_job_created"
	JobHostNetwork = "flynn_job_host_network"
	JobReleaseID   = "flynn_job_release_id"
	JobAppID       = "flynn_job_app_id"
	JobType        = "flynn_job_type"

	StatusStarting = "starting"
	StatusRunning  = "running"
	StatusDone     = "done"
	StatusCrashed  = "crashed"
	StatusFailed   = "failed"
)

// Job is the interface to a specific Flynn job
type Job interface {
	ID() string
	GetNode() report.Node
	PID() int
	Status() string
	UpdateState(host.ActiveJob)
}

type job struct {
	sync.RWMutex
	activeJob host.ActiveJob
	baseNode  report.Node
}

// NewJob returns a new Job wrapper around a Flynn job
func NewJob(activeJob host.ActiveJob) Job {
	j := &job{
		activeJob: activeJob,
	}
	j.baseNode = j.getBaseNode()
	return j
}

func (j *job) ID() string {
	j.RLock()
	defer j.RUnlock()
	return j.activeJob.Job.ID
}

func (j *job) PID() int {
	j.RLock()
	defer j.RUnlock()
	var pid int
	if j.activeJob.PID != nil {
		pid = *j.activeJob.PID
	}
	return pid
}

func (j *job) Status() string {
	j.RLock()
	defer j.RUnlock()
	return j.activeJob.Status.String()
}

func (j *job) HostNetwork() bool {
	j.RLock()
	defer j.RUnlock()
	return j.activeJob.Job.Config.HostNetwork
}

func (j *job) IPs() []string {
	j.RLock()
	defer j.RUnlock()
	// TODO(jpg) handle host networked jobs
	if j.activeJob.InternalIP != "" {
		return []string{j.activeJob.InternalIP}
	}
	return []string{}
}

func (j *job) UpdateState(activeJob host.ActiveJob) {
	j.Lock()
	defer j.Unlock()
	j.activeJob = activeJob
}

func (j *job) Args() []string {
	return []string{}
}

func (j *job) Created() time.Time {
	return j.activeJob.CreatedAt
}

func (j *job) ReleaseID() string {
	if r, ok := j.activeJob.Job.Metadata["flynn-controller.release"]; ok {
		return r
	}
	return ""
}

func (j *job) AppID() string {
	if a, ok := j.activeJob.Job.Metadata["flynn-controller.app"]; ok {
		return a
	}
	return ""
}

func (j *job) Type() string {
	if t, ok := j.activeJob.Job.Metadata["flynn-controller.type"]; ok {
		return t
	}
	return ""
}

func (j *job) getBaseNode() report.Node {
	result := report.MakeNodeWith(report.MakeFlynnJobNodeID(j.ID()), map[string]string{
		JobID:          j.ID(),
		JobArgs:        strings.Join(j.Args(), " "),
		JobCreated:     j.Created().Format(time.RFC3339Nano),
		JobHostNetwork: strconv.FormatBool(j.HostNetwork()),
		JobAppID:       j.AppID(),
		JobReleaseID:   j.ReleaseID(),
		JobType:        j.Type(),
	})

	// TODO(jpg) handle host networked jobs
	if len(j.IPs()) > 0 {
		result = result.WithSets(report.EmptySets.Add(JobIPs, report.MakeStringSet(j.IPs()...)))
	}
	return result
}

// GetNode returns the report for this Job
func (j *job) GetNode() report.Node {
	j.RLock()
	defer j.RUnlock()

	latest := map[string]string{
		JobStatus: j.Status(),
	}

	if j.Status() == StatusRunning {
		uptime := (mtime.Now().Sub(j.Created()) / time.Second) * time.Second
		latest[JobUptime] = uptime.String()
	}

	result := j.baseNode.WithLatests(latest)
	return result
}

func JobIsStopped(j Job) bool {
	// TODO(jpg): Is StatusStarting also valid here? Need to check when flynn-host updates with PID.
	return j.Status() != StatusRunning
}
