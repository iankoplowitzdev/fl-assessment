# README

## Running

`make cycle` and in another terminal `make run`

## Validating

You can shell into the running Postgres container and view the created entries:

`pqsl -U postgres`

`\c nfl_stats`

`SELECT * FROM game_plays;`