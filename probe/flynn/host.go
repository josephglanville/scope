package flynn

import (
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/flynn/flynn/host/types"
	"github.com/flynn/flynn/pkg/cluster"
	"github.com/weaveworks/scope/report"
)

// FlynnHost is the interface to the flynn-host client
type FlynnHost interface {
	Stop()
	WalkJobs(f func(Job))
	WatchJobUpdates(JobUpdateWatcher)
	LockedPIDLookup(f func(func(int) Job))
	GetJob(string) (Job, bool)
}

// JobUpdateWatcher returns the report for a Flynn job
type JobUpdateWatcher func(report.Node)

type flynnHost struct {
	sync.RWMutex
	hc       *cluster.Host
	interval time.Duration
	quit     chan chan struct{}
	watchers []JobUpdateWatcher

	jobs      map[string]Job
	jobsByPID map[int]Job
}

// NewFlynnHost instansiates a flynn-host client
func NewFlynnHost(interval time.Duration, addr string) FlynnHost {
	h := &flynnHost{
		jobs:      map[string]Job{},
		jobsByPID: map[int]Job{},
		interval:  interval,
		quit:      make(chan chan struct{}),
	}
	// TODO(jpg) clarify semantics of error handling
	hc := cluster.NewHost("", addr, nil, nil)
	status, err := hc.GetStatus()
	if err != nil {
		return nil
	}
	// Reconnect with info from GetStatus
	h.hc = cluster.NewHost(status.ID, addr, nil, status.Tags)
	go h.loop()
	return h
}

// WatchJobUpdates runs f on jobs which have changed state
func (h *flynnHost) WatchJobUpdates(f JobUpdateWatcher) {
	h.Lock()
	defer h.Unlock()
	h.watchers = append(h.watchers, f)
}

func (h *flynnHost) LockedPIDLookup(f func(func(int) Job)) {
	h.RLock()
	defer h.RUnlock()

	lookup := func(pid int) Job {
		return h.jobsByPID[pid]
	}

	f(lookup)
}

// WalkJobs applies f to each job
func (h *flynnHost) WalkJobs(f func(Job)) {
	h.RLock()
	defer h.RUnlock()

	for _, j := range h.jobs {
		f(j)
	}
}

// GetJob returns a job by ID
func (h *flynnHost) GetJob(id string) (Job, bool) {
	h.RLock()
	defer h.RUnlock()

	j, ok := h.jobs[id]
	if ok {
		return j, true
	}

	return nil, false
}

// Stop shuts down the flynn host event listener
func (h *flynnHost) Stop() {
	ch := make(chan struct{})
	h.quit <- ch
	<-ch
}

func (h *flynnHost) loop() {
	for {
		// Exit loop if false is returned from listenForEvents
		if !h.listenForEvents() {
			return
		}
		time.Sleep(h.interval)
	}
}

func (h *flynnHost) reset() {
	h.Lock()
	defer h.Unlock()

	h.jobs = map[string]Job{}
	h.jobsByPID = map[int]Job{}
}

func (h *flynnHost) updateJobs() error {
	log.Info("fetching job list from flynn-host")
	jobs, err := h.hc.ListJobs()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		h.updateJobState(j)
	}
	return nil
}

func (h *flynnHost) updateJobState(activeJob host.ActiveJob) {
	h.Lock()
	defer h.Unlock()

	id := activeJob.Job.ID

	switch activeJob.Status {
	case host.StatusRunning: // TODO(jpg) handle StatusStarting
		j, ok := h.jobs[id]
		if !ok {
			// First time we have seen this job, add it to local store
			new := NewJob(activeJob)
			h.jobs[id] = new
		} else {
			// Potentially clear PID mapping
			delete(h.jobsByPID, j.PID())
			j.UpdateState(activeJob)
		}

		cur := h.jobs[id]

		if cur.PID() > 1 {
			h.jobsByPID[cur.PID()] = cur
		}

		node := cur.GetNode()
		for _, f := range h.watchers {
			f(node)
		}
	case host.StatusDone, host.StatusCrashed, host.StatusFailed:
		j, ok := h.jobs[id]
		if !ok {
			return
		}
		delete(h.jobs, id)
		delete(h.jobsByPID, j.PID())

		node := report.MakeNodeWith(report.MakeFlynnJobNodeID(id), map[string]string{
			JobID:     id,
			JobStatus: j.Status(),
		})
		for _, f := range h.watchers {
			f(node)
		}
	}
}

func (h *flynnHost) listenForEvents() bool {
	// First empty state, ensures clean slate on new invocations of listenForEvents
	h.reset()

	// Listen for events before populating state to ensure no updates are missed.
	events := make(chan *host.Event)
	stream, err := h.hc.StreamEvents("all", events)
	if err != nil {
		log.Errorf("Error streaming events from flynn-host: %s", err)
		return true
	}
	defer stream.Close()

	log.Info("connected to flynn-host event stream")

	// Sync current state
	if err := h.updateJobs(); err != nil {
		log.Errorf("flynn-host: %s", err)
		return true
	}

	for {
		select {
		case event, ok := <-events:
			if !ok {
				log.Errorf("flynn-host event stream unexpectedly closed, err:", stream.Err())
				return true
			}
			h.updateJobState(*event.Job)
		case ch := <-h.quit:
			h.Lock()
			defer h.Unlock()

			close(ch)
			return false
		}
	}
}
