package main

import (
	"time"
)

type EventType string

const (
	EventJobStarted      EventType = "job.started"
	EventJobSucceeded    EventType = "job.succeeded"
	EventJobFailed       EventType = "job.failed"
	EventJobCancelled    EventType = "job.cancelled"
	EventWorkflowStarted EventType = "workflow.started"
	EventWorkflowDone    EventType = "workflow.done"
)

type Event struct {
	Type         EventType
	JobID        string
	WorkflowName string
	Err          error
	Timestamp    time.Time
}

type Subscriber interface {
	OnEvent(Event)
}

type EventBus struct {
	subscribers []Subscriber
}

func (b *EventBus) Subscribe(s Subscriber) {
	b.subscribers = append(b.subscribers, s)
}

func (b *EventBus) Publish(e Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	for _, s := range b.subscribers {
		s.OnEvent(e)
	}
}
