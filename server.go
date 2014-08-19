package kraken

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vincent-petithory/kraken/fileserver"
)

type DirAliases struct {
	m   map[string]fileserver.Server
	mu  sync.RWMutex
	fsf fileserver.Factory
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

// Alias has an invalid value.
var ErrInvalidAlias = errors.New("invalid alias value")

// Put registers an alias for the given path.
// It returns true if the alias already exists.
func (da *DirAliases) Put(alias string, path string, fsType string, fsParams fileserver.Params) (bool, error) {
	da.mu.Lock()
	defer da.mu.Unlock()

	// alias must start with /
	if !strings.HasPrefix(alias, "/") {
		return false, ErrInvalidAlias
	}
	// if alias is not "/" and has a trailing /, reject it
	if alias != "/" && strings.HasSuffix(alias, "/") {
		return false, ErrInvalidAlias
	}
	_, ok := da.m[alias]

	fs := da.fsf.New(path, fsType, fsParams)
	da.m[alias] = fs
	return ok, nil
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
	var (
		maxAliasLen int
		alias       string
	)
	da.mu.RLock()
	for a := range da.m {
		if strings.HasPrefix(r.URL.Path, a) && len(a) >= maxAliasLen {
			maxAliasLen = len(a)
			alias = a
		}
	}
	if maxAliasLen == 0 {
		http.NotFound(w, r)
		da.mu.RUnlock()
		return
	}

	if alias != "/" {
		r.URL.Path = r.URL.Path[maxAliasLen:]
	}
	fs, ok := da.m[alias]
	da.mu.RUnlock()
	if !ok {
		http.Error(w, fmt.Sprintf("alias %q not found", alias), http.StatusNotFound)
		return
	}
	fs.ServeHTTP(w, r)
}

func NewDirAliases(fsf fileserver.Factory) *DirAliases {
	return &DirAliases{
		m:   make(map[string]fileserver.Server),
		fsf: fsf,
	}
}

type Server struct {
	DirAliases *DirAliases
	Addr       string
	Port       uint16
	Started    chan struct{}
	srv        *http.Server
	ln         net.Listener
}

func NewServer(addr string, fsf fileserver.Factory) *Server {
	return &Server{
		DirAliases: NewDirAliases(fsf),
		Addr:       addr,
		Started:    make(chan struct{}),
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

	close(ds.Started)
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

type ServerPool struct {
	Srvs  []*Server
	SrvCh chan *Server
	Fsf   fileserver.Factory
}

func NewServerPool(fsf fileserver.Factory) *ServerPool {
	return &ServerPool{
		Srvs:  make([]*Server, 0),
		SrvCh: make(chan *Server),
		Fsf:   fsf,
	}
}

func (sp *ServerPool) Add(addr string) (*Server, error) {
	if err := checkAddr(addr); err != nil {
		return nil, err
	}
	ds := NewServer(addr, sp.Fsf)
	sp.SrvCh <- ds
	<-ds.Started
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
