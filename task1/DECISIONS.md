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
- Use a subquery query for points lookup

## AI usage and tweaked output
- Initially started with Hello World pipeline and blank processes, updated to mimic NFL Sunday with stat updates

Prompt:

We're going to make updates to the task1 data flow.

First, create a local postgres database instance, and add it to the docker-compose. Configure the daemon to connect to the database on startup. Create an init.sql file to contain nfl weekly scores and stats that are received and processed by the daemon (additional details below)

Instead of the "hello world" update, I want to send nfl game stat updates. The body should include a primary and secondary player, the number of yards the player gained on the play, and the updated score of the game (same score if it didn't change). For this example, let's stay simple with just a quarterback and running back, with passing/rushing yards, and touchdowns.

Update the intermediary stages. Keep the schema validation in place. Update the second step to be a translation step to how many fantasy points the incoming payload translates to. Passing yards are 1 point per 25 yards, rushing yards are 1 point per 10 yards. Touchdowns for both are 6 points. Keep the stat-to-point lookup in the postgres database.

The final step should be to save the incoming stats to the postgres database.