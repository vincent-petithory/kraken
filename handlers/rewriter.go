package handlers

import (
	"bytes"
	"io"
	"net/http"
)

type responseRewriter struct {
	http.ResponseWriter
	r      *http.Request
	rw     Rewriter
	buf    *bytes.Buffer
	ew     io.Writer
	status int
}

func (w *responseRewriter) Write(b []byte) (int, error) {
	return w.ew.Write(b)
}

func (w *responseRewriter) WriteHeader(s int) {
	w.status = s
	if w.rw.RewriteIf(w.Header(), s, w.r) {
		w.buf = new(bytes.Buffer)
		w.ew = w.buf
		if hrw, ok := w.rw.(HeaderRewriter); ok {
			hrw.RewriteHeader(w.Header(), s)
		}
	}
	w.ResponseWriter.WriteHeader(s)
}

func (w *responseRewriter) Close() {
	if w.buf != nil {
		w.rw.Rewrite(w.ResponseWriter, w.buf.Bytes(), w.status)
	}
}

type (
	Rewriter interface {
		RewriteIf(header http.Header, status int, r *http.Request) bool
		Rewrite(w io.Writer, b []byte, status int)
	}

	HeaderRewriter interface {
		RewriteHeader(header http.Header, status int)
	}

	ResponseRewriter interface {
		Rewriter
		HeaderRewriter
	}
)

func ResponseRewriteHandler(rw Rewriter, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rrw := &responseRewriter{
			ResponseWriter: w,
			r:              r,
			rw:             rw,
			ew:             w,
		}
		h.ServeHTTP(rrw, r)
		rrw.Close()
	})
}
