package kraken

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

type DirAliases struct {
	m                 map[string]FileServer
	mu                sync.RWMutex
	FileServerCreator FileServerCreator
}

func (da *DirAliases) List() []string {
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
func (da *DirAliases) Get(alias string) string {
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
func (da *DirAliases) Put(alias string, path string) bool {
	da.mu.Lock()
	defer da.mu.Unlock()
	_, ok := da.m[alias]

	fs := da.FileServerCreator(path)
	da.m[alias] = fs
	return ok
}

// Delete removes an existing alias.
// It returns true if the alias existed.
func (da *DirAliases) Delete(alias string) bool {
	da.mu.RLock()
	defer da.mu.RUnlock()
	_, ok := da.m[alias]
	delete(da.m, alias)
	return ok
}

func (da *DirAliases) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func NewDirAliases() *DirAliases {
	return &DirAliases{
		m:                 make(map[string]FileServer),
		FileServerCreator: StdFileServerCreator,
	}
}

type Server struct {
	DirAliases *DirAliases
	Addr       string
	Port       uint16
	srv        *http.Server
	ln         net.Listener
	started    chan struct{}
}

func NewServer(addr string) *Server {
	return &Server{
		DirAliases: NewDirAliases(),
		Addr:       addr,
		started:    make(chan struct{}),
	}
}

func (ds *Server) ListenAndServe() error {
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
	ds.ln = &connsCloserListener{
		Listener: tcpKeepAliveListener{ln.(*net.TCPListener)},
	}

	close(ds.started)
	if err := ds.srv.Serve(ds.ln); err != nil {
		return err
	}
	return nil
}

func (ds *Server) Close() error {
	return ds.ln.Close()
}

type connsCloserListener struct {
	net.Listener
	conns []net.Conn
}

func (ln *connsCloserListener) Accept() (c net.Conn, err error) {
	c, err = ln.Listener.Accept()
	if err != nil {
		return
	}
	ln.conns = append(ln.conns, c)
	return c, nil
}

func (ln *connsCloserListener) Close() error {
	for _, c := range ln.conns {
		if err := c.Close(); err != nil {
			log.Println(err)
		}
	}
	ln.conns = nil
	return ln.Listener.Close()
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

func NewServerPool() *ServerPool {
	return &ServerPool{
		Srvs:  make([]*Server, 0),
		SrvCh: make(chan *Server),
	}
}

type ServerPool struct {
	Srvs  []*Server
	SrvCh chan *Server
}

func (sp *ServerPool) Add(addr string) (*Server, error) {
	if err := checkAddr(addr); err != nil {
		return nil, err
	}
	ds := NewServer(addr)
	sp.SrvCh <- ds
	<-ds.started
	sp.Srvs = append(sp.Srvs, ds)
	return ds, nil
}

func (sp *ServerPool) Get(port uint16) *Server {
	for _, srv := range sp.Srvs {
		if srv.Port == port {
			return srv
		}
	}
	return nil
}

func (sp *ServerPool) Remove(port uint16) (bool, error) {
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

func (sp *ServerPool) ListenAndRun() {
	for _, srv := range sp.Srvs {
		go func(ds *Server) {
			// TODO remove server from list?
			log.Print(ds.ListenAndServe())
		}(srv)
	}
	for srv := range sp.SrvCh {
		go func(ds *Server) {
			// TODO remove server from list?
			log.Print(ds.ListenAndServe())
		}(srv)
	}
}

func checkAddr(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return ln.Close()
}
