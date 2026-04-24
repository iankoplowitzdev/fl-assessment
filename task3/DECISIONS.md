# Decisions

DECISIONS.md in the repo root covering: your key architectural choices and which patterns you used and why, what alternatives you considered and rejected, and where you used AI and what you changed from its output

## Key architectural choices
- Rate limit the client before the retry - this ensures the retries don't pile up without respecting limiter parameters


## Patterns used and why


## Alternatives
- The current implementation of the decorator pattern doesn't enforce ordering. This is good and bad. I can see the value in the developer being able to re-order ad hoc, but it also wouldn't make a ton of sense to have the rate limiter logic execute after the retryer, or the logger happen first. Enforcing some sort of chain ordering system would be good. This would allow futher implementations to use the Doer chain, with varying decorators, but still benefit from them being wrapped correctly.

## AI usage and tweaked output
- Interesting item here was the AI use of mutex locking the cache. Since the doer is meant to be initialized a single time, and utilized by all outgoing requests, the in-memory cache needs to lock due to Go map's non concurrent-safe nature
- The AI cache was only accounting for 200-299 success responses. I could see scenarios where we'd potentially want to cache other response codes (404?)
