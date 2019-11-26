package main

import (
	"fmt"
	"sync"
	"time"
)

const (
	JOB_INTERVAL = 20 * time.Second // How often the job should run
)

type RecurringJob struct {
	cancel     chan struct{}
	cancelled  chan struct{}
	cancelOnce sync.Once
	plugin     *Plugin
}

func (p *Plugin) InitRecurringJob(enable bool) {
	// Config is set to enable. No job exists, start a new job.
	if enable && p.recurringJob == nil {
		job := newRecurringJob(p)
		p.recurringJob = job
		job.Start()
	}

	// Config is set to disable. Job exists, kill existing job.
	if !enable && p.recurringJob != nil {
		p.recurringJob.Cancel()
		p.recurringJob = nil
	}
}

func (job *RecurringJob) Start() {
	fmt.Println("Cronofy Plugin job starting.")

	go func() {
		defer close(job.cancelled)

		ticker := time.NewTicker(JOB_INTERVAL)
		defer func() {
			ticker.Stop()
		}()

		for {
			select {
			case <-ticker.C:
				job.Run()
			case <-job.cancel:
				return
			}
		}
	}()
}

func (job *RecurringJob) Run() {
	p := job.plugin
	h := &Handler{plugin: p}

	username := "mickmister"
	user, appErr := p.API.GetUserByUsername(username)
	if appErr != nil {
		return
	}

	p.CreateBotDMtoMMUserId(user.Id, "Running availability job")

	res, err := getAvailabiltiesAndUpdateStatus(h, user.Id)
	if err != nil {
		p.CreateBotDMtoMMUserId(user.Id, fmt.Sprintf("Failed to run availability job. %s", err.Error()))
		return
	}

	p.CreateBotDMtoMMUserId(user.Id, fmt.Sprintf("Successfully ran availability job. %s", res))
}

func newRecurringJob(p *Plugin) *RecurringJob {
	return &RecurringJob{
		cancel:    make(chan struct{}),
		cancelled: make(chan struct{}),
		plugin:    p,
	}
}

func (job *RecurringJob) Cancel() {
	job.cancelOnce.Do(func() {
		close(job.cancel)
	})
	<-job.cancelled
}
