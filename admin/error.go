package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	krakenhandlers "github.com/vincent-petithory/kraken/handlers"
)

type APIErrorType string

const (
	apiErrTypeBadRequest  APIErrorType = "bad_request_error"
	apiErrTypeAPIInternal              = "api_internal_error"
)

type APIError struct {
	Type APIErrorType `json:"type"`
	Msg  string       `json:"msg"`
}

func (e *APIError) String() string {
	return fmt.Sprintf("%s: %s", e.Type, e.Msg)
}

func (e *APIError) Error() string {
	return e.Msg
}

// JSONRewriter rewrites responses which are 4xx or 5xx
// into a specific error type a JSON REST API could use for its endpoints.
type JSONRewriter struct {
	sph *ServerPoolHandler
}

func (jr JSONRewriter) RewriteIf(header http.Header, status int, r *http.Request) bool {
	return status >= 400 /* 4xx and 5xx */ &&
		!strings.HasPrefix(header.Get("Content-Type"), "application/json") &&
		(r.Method != "HEAD" && r.Method != "OPTIONS")
}

func (jr JSONRewriter) RewriteHeader(header http.Header, status int) {
	header.Set("Content-Type", "application/json; charset=utf-8")
}

func (jr JSONRewriter) Rewrite(w io.Writer, b []byte, status int) {
	aerr := APIError{Msg: string(b)}
	if status >= 400 && status < 500 {
		aerr.Type = apiErrTypeBadRequest
	} else if status >= 500 {
		aerr.Type = apiErrTypeAPIInternal
	}
	b, err := json.MarshalIndent(aerr, "", "  ")
	if err != nil {
		jr.sph.logErr(err)
		return
	}
	fmt.Fprint(w, string(b))
}

func jsonResponseRewriteHandler(sph *ServerPoolHandler, h http.Handler) http.Handler {
	return krakenhandlers.ResponseRewriteHandler(
		JSONRewriter{sph},
		h,
	)
}
