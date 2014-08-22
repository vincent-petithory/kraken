package kraken_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/vincent-petithory/kraken"
	"github.com/vincent-petithory/kraken/fileserver"
)

type mockFileServer struct {
	http.Handler
	RootFn func() string
}

func (fs mockFileServer) Root() string {
	return fs.RootFn()
}

func TestMountMapHandler(t *testing.T) {
	fsf := make(fileserver.Factory)
	if err := fsf.Register("mock", fileserver.Constructor(func(root string, params fileserver.Params) fileserver.Server {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, r.URL.Path)
		})
		return &mockFileServer{
			Handler: h,
			RootFn: func() string {
				return root
			},
		}
	})); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		Target          string
		ReqPath         string
		ExpectedURLPath string
		Status          int
	}{
		{"/foo", "/foo/bar", "/bar", http.StatusOK},
		{"/baz", "/baz/", "/", http.StatusOK},
		{"/", "/home/meow/Public", "/home/meow/Public", http.StatusOK},
		{"/bar", "/meow", "/", http.StatusNotFound},
	}
	mountSource, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		da := kraken.NewMountMap(fsf)
		_, err := da.Put(test.Target, mountSource, "mock", nil)
		if err != nil {
			t.Error(err)
			return
		}

		w := httptest.NewRecorder()
		r, err := http.NewRequest("GET", fmt.Sprintf(test.ReqPath), nil)
		if err != nil {
			t.Fatal(err)
		}
		da.ServeHTTP(w, r)

		if w.Code != test.Status {
			t.Errorf("expected http status %d, got %d", test.Status, w.Code)
			continue
		}
		if w.Code != http.StatusOK {
			continue
		}
		urlPath := w.Body.String()
		if urlPath != test.ExpectedURLPath {
			t.Errorf("expected %v, got %v", test.ExpectedURLPath, urlPath)
		}
	}
}
