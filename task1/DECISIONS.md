# Decisions

DECISIONS.md in the repo root covering: your key architectural choices and which patterns you used and why, what alternatives you considered and rejected, and where you used AI and what you changed from its output

## Key architectural choices
- SQS for at-least-once delivery. Built-in deadletter queueing for failed processing steps
- Got to a point where I had ingestion able pull messages from SQS queue with configurable stages
- In-ordering processing was not a requirement for the assessment, so I introduced async goroutines with wait groups to process the data faster


## Patterns used and why
- Interface for stage allows me to outlined structured nature of the lifestyle hooks
- Stage setup is performed 

## Alternatives

## AI usage and tweaked output