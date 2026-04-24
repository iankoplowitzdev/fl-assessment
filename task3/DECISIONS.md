# Decisions

DECISIONS.md in the repo root covering: your key architectural choices and which patterns you used and why, what alternatives you considered and rejected, and where you used AI and what you changed from its output

## Key architectural choices
- Rate limit the client before the retry - this ensures the retries don't pile up without respecting limiter parameters
- I'd update the auth middleware to validate incoming JWT tokens against a JWKS endpoint. In a larger scale distributed system, this would actually likely occur at the edge/api gateway layer, and unauthenticated/forbidden requests wouldn't even make it to the service. The auth middleware could then just focus on ensuring the JWT payload was passed from the upstream validation layer.
- I'd add a telemetry monitoring middleware to the top of the server middleware chain to monitor traffic for the service
- Generally good rule of thumb is to have central middleware for all routes (authentication, logging, etc...) declared in the main file when the api is setup. We can them also curate a variaty of more niche middlewares, house them in the internal/ directory, and apply them on a per route basis. This can apply to things like ensuring a specific permission is on the incoming JWT, ensuring a specific api key is provided, etc...

## Patterns used and why
- This was essentially an exercise in the decorator Golang pattern, and middleware chaining, which is a standard convention we see for HTTP servers.

## Alternatives
- The current implementation of the decorator pattern doesn't enforce ordering. This is good and bad. I can see the value in the developer being able to re-order ad hoc, but it also wouldn't make a ton of sense to have the rate limiter logic execute after the retryer, or the logger happen first. Enforcing some sort of chain ordering system would be good. This would allow futher implementations to use the Doer chain, with varying decorators, but still benefit from them being wrapped correctly.

## AI usage and tweaked output
- Interesting item here was the AI use of mutex locking the cache. Since the doer is meant to be initialized a single time, and utilized by all outgoing requests, the in-memory cache needs to lock due to Go map's non concurrent-safe nature
- The AI cache was only accounting for 200-299 success responses. I could see scenarios where we'd potentially want to cache other response codes (404?)
- This was a super interesting case study. I have used the decorator and middleware patterns, but it was interesting to see how Claude foritified against things like internal data maniuplation for me. A classic case of the AI bringing to light issues its trained on that I hadn't encountered before. (e.g. client retryer)
- Yeah similar mutex locking for the server side rate limiting