package fileserver

import (
	"errors"
	"fmt"
	"net/http"
)

type Server interface {
	http.Handler
	Root() string
}

type Params map[string]interface{}

type Factory map[string]Constructor

func (f Factory) New(root string, typ string, params Params) Server {
	for typeName, constructor := range f {
		if typ == typeName {
			return constructor(root, params)
		}
	}
	return defaultConstructor(root, params)
}

func (f Factory) Register(name string, constructor Constructor) error {
	if name == "" {
		return errors.New("fileserver: name is empty")
	}
	if constructor == nil {
		return errors.New("fileserver: constructor is nil")
	}
	if _, ok := f[name]; ok || name == "default" {
		return fmt.Errorf("fileserver: type %q is registered", name)
	}
	f[name] = constructor
	return nil
}

func (f Factory) Types() []string {
	types := make([]string, 0, len(f)+1)
	for typ := range f {
		types = append(types, typ)
	}
	types = append(types, "default")
	return types
}

type defaultServer struct {
	http.Handler
	root string
}

func (fs defaultServer) Root() string {
	return fs.root
}

var defaultConstructor Constructor = func(root string, params Params) Server {
	return &defaultServer{
		Handler: http.FileServer(http.Dir(root)),
		root:    root,
	}
}

type Constructor func(root string, params Params) Server
