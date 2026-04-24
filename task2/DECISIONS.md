# Decisions

DECISIONS.md in the repo root covering: your key architectural choices and which patterns you used and why, what alternatives you considered and rejected, and where you used AI and what you changed from its output

## Key architectural choices
- The main.go file here should be used to read in env variables, read and create the workflow from the workflow.json file, set up the event bus and its subscribers
- Doing a topological sort based on the workflow json configuration is a great way to allow the manifest to be flexible, allowing new jobs to be inserted at any time, and providing a sanity check that no cycles exist in the workflow
- Toggleable and configurable retry centralized to the workflow file, allowing engineer to tweak values as they see fit
- After create a synchronous DAG, I expended the functionality to be able to run DAG nodes in goroutines, as long as their dependencies have been met. This was done by adding a job to write the transformed stats to a postgres database, alongside emailing it to users.

## Patterns used and why


## Alternatives
- This aligns similarly to a AWS StepFuncion project I worked on. Logic could likely be approached in a similar manner using SAM, Lambda, and Step Functions


## AI usage and tweaked output
- This ended up being a fairly complex prompt engineering solution
- Wrote out a HARNESS.md file that seeded the initial DAG for this work, and iterated via claude agent from there
- One thing I really like is the AI's solution doesn't require an arbitrary entry point; if a job has no dependencies, it will start running when the workflow runs
  - This means there is support for singleton jobs that have no dependencies or have any other jobs depending on it

## Currently outstanding questions
- How do we handle a job later in the DAG that has multiple branches converge back into it if a job fails? I.e., what happens with the other branch that's already completed? I'd imagine we'd design the DAG in such a way that the other branch's work wouldn't need to be erased (i.e. the other branch would be a processing job or fetch of some sort). This way it's basically idempotent from run to run