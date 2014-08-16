package main

import (
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type dirAliases struct {
	M map[string]string
	sync.RWMutex
}

func (da *dirAliases) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		DirAliases: &dirAliases{M: make(map[string]string)},
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
