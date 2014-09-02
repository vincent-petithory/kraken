package handlers_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vincent-petithory/kraken/handlers"
)

type rewriterTest struct {
	RewriteIfFn     func(header http.Header, status int) bool
	RewriteFn       func(w io.Writer, b []byte, status int)
	RewriteHeaderFn func(header http.Header, status int)
}

func (rw rewriterTest) RewriteIf(header http.Header, status int, r *http.Request) bool {
	if rw.RewriteIfFn != nil {
		return rw.RewriteIfFn(header, status)
	}
	return false
}

func (rw rewriterTest) Rewrite(w io.Writer, b []byte, status int) {
	if rw.RewriteFn == nil {
		panic("w.Rewrite is nil")
	}
	rw.RewriteFn(w, b, status)
}

func (rw rewriterTest) RewriteHeader(header http.Header, status int) {
	if rw.RewriteHeaderFn != nil {
		rw.RewriteHeaderFn(header, status)
	}
}

func TestRewriter(t *testing.T) {
	tests := []struct {
		code       int
		body       string
		rw         handlers.Rewriter
		respCode   int
		respHeader http.Header
		respBody   string
	}{
		{
			http.StatusInternalServerError,
			http.StatusText(http.StatusInternalServerError),
			rewriterTest{
				RewriteIfFn: func(header http.Header, status int) bool {
					return status >= 500
				},
				RewriteHeaderFn: func(header http.Header, status int) {
					header.Set("Content-Type", "application/json")
				},
				RewriteFn: func(w io.Writer, b []byte, status int) {
					fmt.Fprintf(w, `{"error": "%s"}`, b)
				},
			},
			http.StatusInternalServerError,
			http.Header{"Content-Type": []string{"application/json"}},
			fmt.Sprintf(`{"error": "%s"}`, http.StatusText(http.StatusInternalServerError)),
		},
	}
	for i, test := range tests {
		h := handlers.ResponseRewriteHandler(test.rw, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(test.code)
			w.Write([]byte(test.body))
		}))
		w := httptest.NewRecorder()
		r, err := http.NewRequest("GET", "/", nil)
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/plain")

		h.ServeHTTP(w, r)
		if w.Code != test.respCode {
			t.Errorf("%d: expected code %d, got %d", i, test.respCode, w.Code)
		}
		if body := w.Body.String(); body != test.respBody {
			t.Errorf("%d: expected body %q, got %q", i, test.respBody, body)
		}
		for k := range test.respHeader {
			th := test.respHeader.Get(k)
			eh := w.HeaderMap.Get(k)
			if th != eh {
				t.Errorf("%d, header %s: expected value %q, got %q", i, k, th, eh)
			}
		}
	}
}
