package client

import "net/http"

// HttpDoer is the composable interface every client decorator implements.
// *http.Client satisfies this interface directly.
type HttpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}
