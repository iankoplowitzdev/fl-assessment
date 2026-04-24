package main

import "log"

// LoggerSubscriber logs lifecycle events using the standard logger.
type LoggerSubscriber struct {
	l *log.Logger
}

func NewLoggerSubscriber(l *log.Logger) *LoggerSubscriber {
	return &LoggerSubscriber{l: l}
}

func (ls *LoggerSubscriber) OnEvent(e Event) {
	switch e.Type {
	case EventWorkflowStarted:
		ls.l.Printf("[workflow] %q started", e.WorkflowName)
	case EventWorkflowDone:
		if e.Err != nil {
			ls.l.Printf("[workflow] %q failed: %v", e.WorkflowName, e.Err)
		} else {
			ls.l.Printf("[workflow] %q completed successfully", e.WorkflowName)
		}
	case EventJobStarted:
		ls.l.Printf("[job] %q → RUNNING", e.JobID)
	case EventJobSucceeded:
		ls.l.Printf("[job] %q → SUCCEEDED", e.JobID)
	case EventJobFailed:
		ls.l.Printf("[job] %q → FAILED: %v", e.JobID, e.Err)
	case EventJobCancelled:
		ls.l.Printf("[job] %q → CANCELLED (upstream failure)", e.JobID)
	}
}
