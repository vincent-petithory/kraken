package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type dirAliases struct {
	m                 map[string]FileServer
	mu                sync.RWMutex
	FileServerFactory fileServerFactory
}

func (da *dirAliases) List() []string {
	da.mu.RLock()
	defer da.mu.RUnlock()
	aliases := make([]string, 0, len(da.m))
	for alias := range da.m {
		aliases = append(aliases, alias)
	}
	return aliases
}

// Get retrieves the path for the given alias.
// It returns "" if the alias doesn't exist.
func (da *dirAliases) Get(alias string) string {
	da.mu.RLock()
	defer da.mu.RUnlock()
	fs, ok := da.m[alias]
	if !ok {
		return ""
	}
	return fs.Root()
}

// Put registers an alias for the given path.
// It returns true if the alias already exists.
func (da *dirAliases) Put(alias string, path string) bool {
	da.mu.Lock()
	defer da.mu.Unlock()
	_, ok := da.m[alias]

	fs := da.FileServerFactory(path)
	da.m[alias] = fs
	return ok
}

// Delete removes an existing alias.
// It returns true if the alias existed.
func (da *dirAliases) Delete(alias string) bool {
	da.mu.RLock()
	defer da.mu.RUnlock()
	_, ok := da.m[alias]
	delete(da.m, alias)
	return ok
}

func (da *dirAliases) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !(strings.Count(r.URL.Path, "/") >= 2 && len(r.URL.Path) > 2) {
		http.NotFound(w, r)
		return
	}
	slashIndex := strings.Index(r.URL.Path[1:], "/")
	if slashIndex == -1 {
		http.NotFound(w, r)
		return
	}
	alias := r.URL.Path[1 : slashIndex+1]
	r.URL.Path = r.URL.Path[1+slashIndex:]

	da.mu.RLock()
	defer da.mu.RUnlock()
	fs, ok := da.m[alias]
	if !ok {
		http.Error(w, fmt.Sprintf("alias %q not found", alias), http.StatusNotFound)
		return
	}
	fs.ServeHTTP(w, r)
}

func newDirAliases() *dirAliases {
	return &dirAliases{
		m:                 make(map[string]FileServer),
		FileServerFactory: stdFileServerFactory,
	}
}

type dirServer struct {
	DirAliases *dirAliases
	Addr       string
	Port       uint16
	srv        *http.Server
	ln         net.Listener
	started    chan struct{}
}

func newDirServer(addr string) *dirServer {
	return &dirServer{
		DirAliases: newDirAliases(),
		Addr:       addr,
		started:    make(chan struct{}),
	}
}

func (ds *dirServer) ListenAndServe() error {
	ln, err := net.Listen("tcp", ds.Addr)
	if err != nil {
		return err
	}
	ds.Addr = ln.Addr().String()
	_, sport, err := net.SplitHostPort(ds.Addr)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(sport)
	if err != nil {
		return err
	}
	ds.Port = uint16(port)
	ds.srv = &http.Server{
		Handler: ds.DirAliases,
	}
	ds.ln = tcpKeepAliveListener{ln.(*net.TCPListener)}

	close(ds.started)
	if err := ds.srv.Serve(ds.ln); err != nil {
		return err
	}
	return nil
}

func (ds *dirServer) Close() error {
	return ds.ln.Close()
}

// borrowed from net/http
type tcpKeepAliveListener struct {
	*net.TCPListener
}

// borrowed from net/http
func (ln tcpKeepAliveListener) Accept() (c net.Conn, err error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}

type serverPool struct {
	Srvs  []*dirServer
	SrvCh chan *dirServer
}

func (sp *serverPool) Add(addr string) (*dirServer, error) {
	if err := CheckAddr(addr); err != nil {
		return nil, err
	}
	ds := newDirServer(addr)
	sp.SrvCh <- ds
	<-ds.started
	sp.Srvs = append(sp.Srvs, ds)
	return ds, nil
}

func (sp *serverPool) Get(port uint16) *dirServer {
	for _, srv := range sp.Srvs {
		if srv.Port == port {
			return srv
		}
	}
	return nil
}

func (sp *serverPool) Remove(port uint16) (bool, error) {
	for i, srv := range sp.Srvs {
		if srv.Port != port {
			continue
		}
		if err := srv.Close(); err != nil {
			return false, err
		}
		copy(sp.Srvs[i:], sp.Srvs[i+1:])
		sp.Srvs[len(sp.Srvs)-1] = nil
		sp.Srvs = sp.Srvs[:len(sp.Srvs)-1]
		return true, nil
	}
	return false, nil
}

func (sp *serverPool) ListenAndRun() {
	for _, srv := range sp.Srvs {
		go func(ds *dirServer) {
			// TODO remove server from list?
			log.Print(ds.ListenAndServe())
		}(srv)
	}
	for srv := range sp.SrvCh {
		go func(ds *dirServer) {
			// TODO remove server from list?
			log.Print(ds.ListenAndServe())
		}(srv)
	}
}

func CheckAddr(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return ln.Close()
}
