I want to create a workflow orchestrator for a fantasy football Tuesday morning results workflow.

The orchestrator shall be referred to as TMRW in this document.

TMRW is written in Golang.
The workflow for TMRW is stored in a json file in the repo.
TMRW is a directed acyclic graph comprised of jobs.
Each job must contain states detailing the current status.

You'll need to create a docker-compose file that runs an nginx server, and hosts the json file of NFL matchup scores. Keep the json file simple, supply only 2 game results from NFL games. For each game, create some sample NFL data (passing yards, rushing touchdowns, etc...).

The jobs for this exercies are:
- HTTP GET
- Stat transformer (transform the nfl stat data into fantasy points)
- Emailer (sends the stats as fantasy points to system users)

The DAG for the jobs is as follows (stored in json):
- HTTP GET must be made to the sample file on the nginx server
  - if the call fails, an expoential backoff retry is made until it succeeds
- The HTTP GET jobs passes the results of the call into the stat transformer job
  - The stat transformer job transforms the stats received into fantasy point values
    - the fantasy point values are configured in the json workflow config file for now
      - e.g. 10 rushing yards is 1 point
  - The result of the stat transformer is a summary of all matchups from the weekend, with the corresponding point values
- The stat transformer passes the summary to an email job
  - the email job asynchronously emails a list of system users. For now, store the system users in the json config file as well

- All jobs must maintain a status of (PENDING, RUNNING, SUCCEEDED/FAILED/CANCELLED)
  - all jobs start in pending, transition to RUNNING while processing, and transition to one of the final three statuses upon completion or error.





