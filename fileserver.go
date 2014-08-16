package main

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

var stdFileServerFactory fileServerFactory = func(root string) FileServer {
	return &stdFileServer{
		Handler: http.FileServer(http.Dir(root)),
		root:    root,
	}
}

type fileServerFactory func(root string) FileServer
