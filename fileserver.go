package kraken

import (
	"net/http"
)

type FileServer interface {
	http.Handler
	Root() string
}

type stdFileServer struct {
	http.Handler
	root string
}

func (fs stdFileServer) Root() string {
	return fs.root
}

var StdFileServerCreator FileServerCreator = func(root string) FileServer {
	return &stdFileServer{
		Handler: http.FileServer(http.Dir(root)),
		root:    root,
	}
}

type FileServerCreator func(root string) FileServer
