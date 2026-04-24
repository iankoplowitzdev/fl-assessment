package main

import (
	"context"
	"fmt"
)

type DAG struct {
	name    string
	jobs    map[string]Job
	deps    map[string][]string
	outputs map[string]any
	bus     *EventBus
}

func NewDAG(name string, bus *EventBus) *DAG {
	return &DAG{
		name:    name,
		jobs:    make(map[string]Job),
		deps:    make(map[string][]string),
		outputs: make(map[string]any),
		bus:     bus,
	}
}

func (d *DAG) AddJob(job Job, dependencies []string) {
	d.jobs[job.ID()] = job
	d.deps[job.ID()] = dependencies
}

// topologicalSort returns job IDs in a valid execution order via Kahn's algorithm.
func (d *DAG) topologicalSort() ([]string, error) {
	for id, deps := range d.deps {
		for _, dep := range deps {
			if _, ok := d.jobs[dep]; !ok {
				return nil, fmt.Errorf("job %q depends on unknown job %q", id, dep)
			}
		}
	}

	inDegree := make(map[string]int, len(d.jobs))
	for id := range d.jobs {
		inDegree[id] = len(d.deps[id])
	}

	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	var order []string
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		order = append(order, curr)

		for id, deps := range d.deps {
			for _, dep := range deps {
				if dep == curr {
					inDegree[id]--
					if inDegree[id] == 0 {
						queue = append(queue, id)
					}
				}
			}
		}
	}

	if len(order) != len(d.jobs) {
		return nil, fmt.Errorf("cycle detected in DAG")
	}
	return order, nil
}

// Run executes jobs in topological order, cancelling downstream jobs on failure.
func (d *DAG) Run(ctx context.Context) error {
	d.bus.Publish(Event{Type: EventWorkflowStarted, WorkflowName: d.name})

	order, err := d.topologicalSort()
	if err != nil {
		return fmt.Errorf("topological sort: %w", err)
	}

	failed := false

	for _, id := range order {
		job := d.jobs[id]

		if failed {
			job.Cancel()
			d.bus.Publish(Event{Type: EventJobCancelled, JobID: id})
			continue
		}

		input := d.collectInput(id)

		d.bus.Publish(Event{Type: EventJobStarted, JobID: id})
		output, err := job.Run(ctx, input)
		if err != nil {
			d.bus.Publish(Event{Type: EventJobFailed, JobID: id, Err: err})
			failed = true
			continue
		}

		d.outputs[id] = output
		d.bus.Publish(Event{Type: EventJobSucceeded, JobID: id})
	}

	var runErr error
	if failed {
		runErr = fmt.Errorf("one or more jobs failed")
	}
	d.bus.Publish(Event{Type: EventWorkflowDone, WorkflowName: d.name, Err: runErr})
	return runErr
}

// collectInput gathers dependency outputs for a job.
// Single dependency: passes the output directly.
// Multiple dependencies: passes map[depID]any.
func (d *DAG) collectInput(id string) any {
	deps := d.deps[id]
	switch len(deps) {
	case 0:
		return nil
	case 1:
		return d.outputs[deps[0]]
	default:
		m := make(map[string]any, len(deps))
		for _, dep := range deps {
			m[dep] = d.outputs[dep]
		}
		return m
	}
}
