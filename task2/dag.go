package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"
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

type jobResult struct {
	id     string
	output any
	err    error
}

// Run executes the DAG, launching each job as a goroutine the moment all of
// its dependencies have succeeded. Jobs whose dependencies fail are cancelled.
// Independent jobs run concurrently.
func (d *DAG) Run(ctx context.Context) error {
	d.bus.Publish(Event{Type: EventWorkflowStarted, WorkflowName: d.name})

	if _, err := d.topologicalSort(); err != nil {
		return fmt.Errorf("invalid DAG: %w", err)
	}

	// Build reverse adjacency and pending dep counts.
	dependents := make(map[string][]string, len(d.jobs))
	pending := make(map[string]int, len(d.jobs))
	for id, deps := range d.deps {
		pending[id] = len(deps)
		for _, dep := range deps {
			dependents[dep] = append(dependents[dep], id)
		}
	}

	succeeded := make(map[string]bool, len(d.jobs))
	results := make(chan jobResult, len(d.jobs))
	running := 0

	// launch starts a job goroutine. Input is captured before the goroutine
	// starts so all dependency outputs are guaranteed visible.
	launch := func(id string) {
		input := d.collectInput(id)
		job := d.jobs[id]
		d.bus.Publish(Event{Type: EventJobStarted, JobID: id})
		running++
		go func() {
			output, err := d.runWithRetry(ctx, job, input)
			results <- jobResult{id: id, output: output, err: err}
		}()
	}

	// cancelTree cancels id and all of its transitive dependents.
	var cancelTree func(id string)
	cancelTree = func(id string) {
		d.jobs[id].Cancel()
		d.bus.Publish(Event{Type: EventJobCancelled, JobID: id})
		for _, dep := range dependents[id] {
			pending[dep]--
			if pending[dep] == 0 {
				cancelTree(dep)
			}
		}
	}

	// Seed: launch jobs that have no dependencies.
	for id, n := range pending {
		if n == 0 {
			launch(id)
		}
	}

	anyFailed := false
	for running > 0 {
		r := <-results
		running--

		if r.err != nil {
			anyFailed = true
			d.bus.Publish(Event{Type: EventJobFailed, JobID: r.id, Err: r.err})
		} else {
			succeeded[r.id] = true
			d.outputs[r.id] = r.output
			d.bus.Publish(Event{Type: EventJobSucceeded, JobID: r.id})
		}

		for _, dep := range dependents[r.id] {
			pending[dep]--
			if pending[dep] == 0 {
				if d.depsSucceeded(dep, succeeded) {
					launch(dep)
				} else {
					cancelTree(dep)
				}
			}
		}
	}

	var runErr error
	if anyFailed {
		runErr = fmt.Errorf("one or more jobs failed")
	}
	d.bus.Publish(Event{Type: EventWorkflowDone, WorkflowName: d.name, Err: runErr})
	return runErr
}

func (d *DAG) runWithRetry(ctx context.Context, job Job, input any) (any, error) {
	policy := job.RetryPolicy()
	maxAttempts := 1
	if policy.Enabled && policy.MaxAttempts > 1 {
		maxAttempts = policy.MaxAttempts
	}
	scalar := policy.BackoffScalar
	if scalar <= 0 {
		scalar = 2.0
	}

	var (
		output any
		err    error
	)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		output, err = job.Run(ctx, input)
		if err == nil {
			return output, nil
		}
		if attempt < maxAttempts-1 {
			delay := time.Duration(math.Pow(scalar, float64(attempt))) * time.Second
			log.Printf("[dag] job %q attempt %d/%d failed, retrying in %v: %v",
				job.ID(), attempt+1, maxAttempts, delay, err)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return nil, err
}

func (d *DAG) depsSucceeded(id string, succeeded map[string]bool) bool {
	for _, dep := range d.deps[id] {
		if !succeeded[dep] {
			return false
		}
	}
	return true
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
