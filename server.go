package kraken

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vincent-petithory/kraken/fileserver"
)

type MountMap struct {
	m   map[string]fileserver.Server
	mu  sync.RWMutex
	fsf fileserver.Factory
}

func (mm *MountMap) Targets() []string {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	mountTargets := make([]string, 0, len(mm.m))
	for mountTarget := range mm.m {
		mountTargets = append(mountTargets, mountTarget)
	}
	return mountTargets
}

// Get retrieves the source for the given mount target.
// It returns "" if the mount target doesn't exist.
func (mm *MountMap) GetSource(mountTarget string) string {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	fs, ok := mm.m[mountTarget]
	if !ok {
		return ""
	}
	return fs.Root()
}

var (
	// ErrInvalidMountTarget describes an invalid value for a mount target.
	ErrInvalidMountTarget = errors.New("invalid mount target value")
	// ErrInvalidMountSource describes an invalid value for a mount source.
	ErrInvalidMountSource = errors.New("invalid mount source value")
)

type MountSourcePermError struct {
	err error
}

func (e *MountSourcePermError) Error() string {
	return e.err.Error()
}

// Put registers a mount target for the given mount source.
// It returns true if the mount target already exists.
func (mm *MountMap) Put(mountTarget string, mountSource string, fsType string, fsParams fileserver.Params) (bool, error) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// mountTarget must start with /
	if !strings.HasPrefix(mountTarget, "/") {
		return false, ErrInvalidMountTarget
	}
	// if mountTarget is not "/" and has a trailing /, reject it
	if mountTarget != "/" && strings.HasSuffix(mountTarget, "/") {
		return false, ErrInvalidMountTarget
	}

	if !path.IsAbs(mountSource) {
		return false, ErrInvalidMountSource
	}

	fi, err := os.Stat(mountSource)
	if err != nil {
		return false, &MountSourcePermError{err}
	}
	if !fi.IsDir() {
		return false, &MountSourcePermError{fmt.Errorf("%s: not a directory", mountSource)}
	}

	_, ok := mm.m[mountTarget]

	fs := mm.fsf.New(mountSource, fsType, fsParams)
	mm.m[mountTarget] = fs
	return ok, nil
}

// Delete removes an existing mount target.
// It returns true if the mount target existed.
func (mm *MountMap) DeleteTarget(mountTarget string) bool {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	_, ok := mm.m[mountTarget]
	delete(mm.m, mountTarget)
	return ok
}

func (mm *MountMap) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var (
		maxMountTargetLen int
		mountTarget       string
	)
	mm.mu.RLock()
	for t := range mm.m {
		if strings.HasPrefix(r.URL.Path, t) && len(t) >= maxMountTargetLen {
			maxMountTargetLen = len(t)
			mountTarget = t
		}
	}
	if maxMountTargetLen == 0 {
		http.NotFound(w, r)
		mm.mu.RUnlock()
		return
	}

	if mountTarget != "/" {
		r.URL.Path = r.URL.Path[maxMountTargetLen:]
	}
	fs, ok := mm.m[mountTarget]
	mm.mu.RUnlock()
	if !ok {
		http.Error(w, fmt.Sprintf("mount target %q not found", mountTarget), http.StatusNotFound)
		return
	}
	fs.ServeHTTP(w, r)
}

func NewMountMap(fsf fileserver.Factory) *MountMap {
	return &MountMap{
		m:   make(map[string]fileserver.Server),
		fsf: fsf,
	}
}

type Server struct {
	MountMap *MountMap
	Addr     string
	Port     uint16
	Started  chan struct{}
	srv      *http.Server
	ln       net.Listener
}

func NewServer(addr string, fsf fileserver.Factory) *Server {
	return &Server{
		MountMap: NewMountMap(fsf),
		Addr:     addr,
		Started:  make(chan struct{}),
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
		Handler: ds.MountMap,
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
