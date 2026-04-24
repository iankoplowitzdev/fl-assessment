# Decisions

DECISIONS.md in the repo root covering: your key architectural choices and which patterns you used and why, what alternatives you considered and rejected, and where you used AI and what you changed from its output

## Key architectural choices
- The main.go file here should be used to read in env variables, read and create the workflow from the workflow.json file, set up the event bus and its subscribers
- Doing a topological sort based on the workflow json configuration is a great way to allow the manifest to be flexible, allowing new jobs to be inserted at any time, and providing a sanity check that no cycles exist in the workflow

## Patterns used and why


## Alternatives



## AI usage and tweaked output
- This ended up being a fairly complex prompt engineering solution
- Wrote out a HARNESS.md file that seeded the initial DAG for this work, and iterated via claude agent from there
- 