# Decisions

DECISIONS.md in the repo root covering: your key architectural choices and which patterns you used and why, what alternatives you considered and rejected, and where you used AI and what you changed from its output

## Key architectural choices
- SQS for at-least-once delivery. Built-in deadletter queueing for failed processing steps
- Got to a point where I had ingestion able pull messages from SQS queue with configurable stages
- In-ordering processing was not a requirement for the assessment, so I introduced async goroutines with wait groups to process the data faster
  - Thought here is, the fantasy points will normalize regardless of when they occur.


## Patterns used and why
- Interface for stage allows me to outlined structured nature of the lifestyle hooks
- Stage setup is performed 
- The added `msgCtx := context.WithoutCancel(ctx)` when processing the messages is an interesting choice. If the shutdown signal is received, do we opt to cancel the in-progress data and leave it on the queue, or finish the DB write and delete the message from the queue. AI is opting for the latter, but I'd almost rather the former. Leaving it on the queue is harmless since we have a data constraint on the message ID. It's possible as the flow expanded, we'd need to do additional cleanup for prior stages, and it would be more advantageous to let the process finish.

## Alternatives
- Use a subquery query for points lookup
- Current daemon is processing 100 plays in ~10 seconds. Is this speed really necessary? NFL plays, even during Sunday 1pm slot don't happen this fast. Could be an area for optimization.

## AI usage and tweaked output
- Initially started with Hello World pipeline and blank processes, updated to mimic NFL Sunday with stat updates
  Prompt:

  We're going to make updates to the task1 data flow.

  First, create a local postgres database instance, and add it to the docker-compose. Configure the daemon to connect to the database on startup. Create an init.sql file to contain nfl weekly scores and stats that are received and processed by the daemon (additional details below)

  Instead of the "hello world" update, I want to send nfl game stat updates. The body should include a primary and secondary player, the number of yards the player gained on the play, and the updated score of the game (same score if it didn't change). For this example, let's stay simple with just a quarterback and running back, with passing/rushing yards, and touchdowns.

  Update the intermediary stages. Keep the schema validation in place. Update the second step to be a translation step to how many fantasy points the incoming payload translates to. Passing yards are 1 point per 25 yards, rushing yards are 1 point per 10 yards. Touchdowns for both are 6 points. Keep the stat-to-point lookup in the postgres database.

  The final step should be to save the incoming stats to the postgres database.

- Used agent to scan and inspect for areas that could lack data integrity:
  Prompt:

  Examine the daemon flow and look for areas that lack data integrity. We need to ensure all messages from the SQS queue are processed, though order does not matter. If there's a failure processing the message, the message should remain on the queue.

  We also want to prioritize speed. This system would be used for fantasy football, where players want updates as fast as possible. Identify areas where the system can run tasks asynchronously, without sacrificing data integrity.

  This yielded the following results:
  - Data duplication concerns if the daemon fails to delete the message from the queue
    - ON CONFLICT added to drop dupes
  - Loading stat_scoring_rules into memory on startup to maximize efficiency
    - This is a false positive. Under a real system, the scoring rules are dynamic per league, and would need to be fetched at process time. That said, the majority of the leagues would be using default settings (or one of a few defaults), so those could be a candidate to load into memory.
  - DB connection pooling
    - This is a big one as we wouldn't want to wait on db connections while we process; can just re-use open ones.

- Used Claude to generate unit tests for the stages file. Did okay with non-database specific ones. For the stage that writes to postgres, I'd recommend a more robust integration test suite that actually verifies the records are correctly written, and clean up afterwards. We're not getting much value out of unit testing as is, since the stage is essentially a data persistence layer for writing to the database. We'd want to ensure the correct records are written.