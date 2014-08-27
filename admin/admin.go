package admin

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/vincent-petithory/kraken"
	"github.com/vincent-petithory/kraken/fileserver"
)

type ServerPoolHandler struct {
	*kraken.ServerPool
	Log    *log.Logger
	h      http.Handler
	routes *ServerPoolRoutes
}

const (
	routeServers               = "servers"
	routeServersSelf           = "servers.self"
	routeServersSelfMounts     = "servers.self.mounts"
	routeServersSelfMountsSelf = "servers.self.mounts.self"
	routeFileServers           = "file-servers"
)

type Route interface {
	URL(*ServerPoolRoutes) (*url.URL, error)
}

type (
	ServersRoute     struct{}
	ServersSelfRoute struct {
		Port uint16
	}
	ServersSelfMountsRoute struct {
		Port uint16
	}
	ServersSelfMountsSelfRoute struct {
		Port uint16
		ID   string
	}
	FileServersRoute struct{}
)

type BadRouteError string

func (e BadRouteError) Error() string {
	return fmt.Sprintf("bad route name and/or parameters: %s", string(e))
}

func (r ServersRoute) URL(spr *ServerPoolRoutes) (*url.URL, error) {
	return spr.url(routeServers)
}
func (r ServersSelfRoute) URL(spr *ServerPoolRoutes) (*url.URL, error) {
	return spr.url(routeServersSelf, "port", strconv.Itoa(int(r.Port)))
}
func (r ServersSelfMountsRoute) URL(spr *ServerPoolRoutes) (*url.URL, error) {
	return spr.url(routeServersSelfMounts, "port", strconv.Itoa(int(r.Port)))
}
func (r ServersSelfMountsSelfRoute) URL(spr *ServerPoolRoutes) (*url.URL, error) {
	return spr.url(routeServersSelfMountsSelf, "port", strconv.Itoa(int(r.Port)), "mount", r.ID)
}
func (r FileServersRoute) URL(spr *ServerPoolRoutes) (*url.URL, error) {
	return spr.url(routeFileServers)
}

type APIErrorType string

const (
	apiErrTypeBadRequest  APIErrorType = "bad_request_error"
	apiErrTypeAPIInternal              = "api_internal_error"
)

type APIError struct {
	Type APIErrorType `json:"type"`
	Msg  string       `json:"msg"`
}

func (e *APIError) String() string {
	return fmt.Sprintf("%s: %s", e.Type, e.Msg)
}

func (e *APIError) Error() string {
	return e.Msg
}

type ServerPoolRoutes struct {
	r       *mux.Router
	BaseURL *url.URL
}

func (spr *ServerPoolRoutes) url(name string, params ...string) (*url.URL, error) {
	var urlPath string
	if u, err := spr.r.Get(name).URLPath(params...); err != nil {
		return nil, BadRouteError(err.Error())
	} else {
		urlPath = u.Path
	}
	var u url.URL
	if spr.BaseURL != nil {
		u = (*spr.BaseURL)
	}
	u.Path = urlPath
	return &u, nil
}

func (spr *ServerPoolRoutes) RouteURL(r Route) (*url.URL, error) {
	return r.URL(spr)
}

func NewServerPoolRoutes() *ServerPoolRoutes {
	r := mux.NewRouter()
	apiRouter := r.PathPrefix("/api/").Subrouter()
	apiRouter.Path("/servers").Name(routeServers)
	apiRouter.Path("/servers/{port:[0-9]{1,5}}").Name(routeServersSelf)
	apiRouter.Path("/servers/{port:[0-9]{1,5}}/mounts").Name(routeServersSelfMounts)
	apiRouter.Path("/servers/{port:[0-9]{1,5}}/mounts/{mount}").Name(routeServersSelfMountsSelf)
	apiRouter.Path("/fileservers").Name(routeFileServers)
	return &ServerPoolRoutes{r: r}
}

func NewServerPoolHandler(serverPool *kraken.ServerPool) *ServerPoolHandler {
	sph := ServerPoolHandler{
		ServerPool: serverPool,
		routes:     NewServerPoolRoutes(),
	}
	sph.routes.r.Get(routeServers).Handler(handlers.MethodHandler{
		"GET":    http.HandlerFunc(sph.getServers),
		"POST":   http.HandlerFunc(sph.createServerWithRandomPort),
		"DELETE": http.HandlerFunc(sph.removeServers),
	})
	sph.routes.r.Get(routeServersSelf).Handler(handlers.MethodHandler{
		"GET":    http.HandlerFunc(sph.getServer),
		"PUT":    http.HandlerFunc(sph.createServer),
		"DELETE": http.HandlerFunc(sph.removeServer),
	})
	sph.routes.r.Get(routeServersSelfMounts).Handler(handlers.MethodHandler{
		"GET":    http.HandlerFunc(sph.getServerMounts),
		"POST":   http.HandlerFunc(sph.createServerMount),
		"DELETE": http.HandlerFunc(sph.removeServerMounts),
	})
	sph.routes.r.Get(routeServersSelfMountsSelf).Handler(handlers.MethodHandler{
		"GET":    http.HandlerFunc(sph.getServerMount),
		"DELETE": http.HandlerFunc(sph.removeServerMount),
	})
	sph.routes.r.Get(routeFileServers).Handler(handlers.MethodHandler{
		"GET": http.HandlerFunc(sph.getFileServers),
	})
	sph.h = sph.routes.r
	return &sph
}

func (sph *ServerPoolHandler) logf(format string, args ...interface{}) {
	if sph.Log != nil {
		sph.Log.Printf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}

func (sph *ServerPoolHandler) logErr(v interface{}) {
	if sph.Log != nil {
		sph.Log.Printf("[error] %v", v)
	} else {
		log.Printf("[error] %v", v)
	}
}

func (sph *ServerPoolHandler) logfSrv(srv *kraken.Server, format string, args ...interface{}) {
	sph.logf(
		fmt.Sprintf("[srv %d] %s", srv.Port, format),
		args...,
	)
}

func (sph *ServerPoolHandler) logErrSrv(srv *kraken.Server, v interface{}) {
	sph.logfSrv(srv, "ERR: %v", v)
}

func (sph *ServerPoolHandler) SetBaseURL(u *url.URL) {
	sph.routes.BaseURL = u
}

func (sph *ServerPoolHandler) BaseURL() *url.URL {
	return &(*(sph.routes.BaseURL))
}

func (sph *ServerPoolHandler) writeLocation(w http.ResponseWriter, route Route) {
	u, err := sph.routes.RouteURL(route)
	if err != nil {
		sph.logErr(err)
		return
	}
	w.Header().Set("Location", u.String())
}

func (sph *ServerPoolHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sph.h.ServeHTTP(w, r)
}

type Server struct {
	BindAddress string  `json:"bind_address"`
	Port        uint16  `json:"port"`
	Mounts      []Mount `json:"mounts"`
}

func newServerDataFromServer(srv *kraken.Server) *Server {
	mountTargets := srv.MountMap.Targets()
	mounts := make([]Mount, 0, len(mountTargets))
	for _, mountTarget := range mountTargets {
		mounts = append(mounts, Mount{
			ID:     mountID(mountTarget),
			Source: srv.MountMap.GetSource(mountTarget),
			Target: mountTarget,
		})
	}
	host, _, _ := net.SplitHostPort(srv.Addr)
	return &Server{
		BindAddress: host,
		Port:        srv.Port,
		Mounts:      mounts,
	}
}

type Mount struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

func mountID(target string) string {
	h := sha1.New()
	h.Write([]byte(target))
	b := h.Sum(nil)
	return fmt.Sprintf("%x", b)[0:7]
}

func (sph *ServerPoolHandler) serverOr404(w http.ResponseWriter, r *http.Request) *kraken.Server {
	sport := mux.Vars(r)["port"]
	port, err := strconv.Atoi(sport)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}
	srv := sph.ServerPool.Get(uint16(port))
	if srv == nil {
		http.Error(w, fmt.Sprintf("server %d not found", port), http.StatusNotFound)
		return nil
	}
	return srv
}

func (sph *ServerPoolHandler) getServers(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	srvs := make([]Server, 0, len(sph.ServerPool.Srvs))
	for _, srv := range sph.ServerPool.Srvs {
		srvs = append(srvs, *newServerDataFromServer(srv))
	}
	if err := json.NewEncoder(w).Encode(srvs); err != nil {
		sph.logErr(err)
	}
}

func (sph *ServerPoolHandler) getServer(w http.ResponseWriter, r *http.Request) {
	srv := sph.serverOr404(w, r)
	if srv == nil {
		return
	}
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(newServerDataFromServer(srv)); err != nil {
		sph.logErr(err)
	}
}

type CreateServerRequest struct {
	BindAddress string `json:"bind_address"`
}

func (sph *ServerPoolHandler) createServerWithRandomPort(w http.ResponseWriter, r *http.Request) {
	var req CreateServerRequest
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	addr := net.JoinHostPort(req.BindAddress, "0")
	srv, err := sph.ServerPool.Add(addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Wait for the server to be started
	<-srv.Started
	sph.logf("created server %q", srv.Addr)
	sph.logfSrv(srv, "server available on http://%s", srv.Addr)

	sph.writeLocation(w, ServersSelfRoute{srv.Port})
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(newServerDataFromServer(srv)); err != nil {
		sph.logErr(err)
	}
}

func (sph *ServerPoolHandler) createServer(w http.ResponseWriter, r *http.Request) {
	var req CreateServerRequest
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sport := mux.Vars(r)["port"]
	port, err := strconv.Atoi(sport)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	addr := net.JoinHostPort(req.BindAddress, strconv.Itoa(port))
	srv, err := sph.ServerPool.Add(addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Wait for the server to be started
	<-srv.Started
	sph.logf("created server %q", srv.Addr)
	sph.logfSrv(srv, "server available on http://%s", srv.Addr)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(newServerDataFromServer(srv)); err != nil {
		sph.logErr(err)
	}
}

func (sph *ServerPoolHandler) removeServers(w http.ResponseWriter, r *http.Request) {
	var errs []error
	srvs := make([]*kraken.Server, len(sph.ServerPool.Srvs))
	copy(srvs, sph.ServerPool.Srvs)
	for _, srv := range srvs {
		if ok, err := sph.ServerPool.Remove(srv.Port); err != nil {
			errs = append(errs, err)
			sph.logErrSrv(srv, err)
		} else if !ok {
			err := fmt.Errorf("unable to shut down server on port %d", srv.Port)
			sph.logErrSrv(srv, "unable to shut down server")
			errs = append(errs, err)
		} else {
			sph.logfSrv(srv, "server shut down")
		}
	}
	if len(errs) > 0 {
		var bufMsg bytes.Buffer
		for _, err := range errs {
			fmt.Fprintln(&bufMsg, err.Error())
		}
		w.WriteHeader(http.StatusInternalServerError)
		http.Error(w, bufMsg.String(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (sph *ServerPoolHandler) removeServer(w http.ResponseWriter, r *http.Request) {
	srv := sph.serverOr404(w, r)
	if srv == nil {
		return
	}
	if ok, err := sph.ServerPool.Remove(srv.Port); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if !ok {
		sph.logErrSrv(srv, "unable to shut down server")
		http.Error(w, fmt.Sprintf("unable to shut down server on port %d", srv.Port), http.StatusInternalServerError)
		return
	} else {
		sph.logfSrv(srv, "server shut down")
	}
	w.WriteHeader(http.StatusOK)
}

func (sph *ServerPoolHandler) getServerMounts(w http.ResponseWriter, r *http.Request) {
	srv := sph.serverOr404(w, r)
	if srv == nil {
		return
	}
	mountTargets := srv.MountMap.Targets()
	mounts := make([]Mount, 0, len(mountTargets))
	for _, mountTarget := range mountTargets {
		mounts = append(mounts, Mount{
			ID:     mountID(mountTarget),
			Source: srv.MountMap.GetSource(mountTarget),
			Target: mountTarget,
		})
	}
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(mounts); err != nil {
		sph.logErr(err)
	}
}

func (sph *ServerPoolHandler) removeServerMounts(w http.ResponseWriter, r *http.Request) {
	srv := sph.serverOr404(w, r)
	if srv == nil {
		return
	}
	mountTargets := srv.MountMap.Targets()
	for _, mountTarget := range mountTargets {
		if ok := srv.MountMap.DeleteTarget(mountTarget); ok {
			sph.logfSrv(srv, "removed mount point %s", mountID(mountTarget))
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (sph *ServerPoolHandler) getServerMount(w http.ResponseWriter, r *http.Request) {
	srv := sph.serverOr404(w, r)
	if srv == nil {
		return
	}
	reqMountID := mux.Vars(r)["mount"]
	var mountTarget string
	for _, mt := range srv.MountMap.Targets() {
		if mountID(mt) == reqMountID {
			mountTarget = mt
			break
		}
	}
	mountSource := srv.MountMap.GetSource(mountTarget)
	if mountSource == "" {
		http.Error(w, fmt.Sprintf("server %d has no mount target %q", srv.Port, mountTarget), http.StatusNotFound)
		return
	}

	mount := Mount{
		ID:     mountID(mountTarget),
		Source: srv.MountMap.GetSource(mountTarget),
		Target: mountTarget,
	}
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(mount); err != nil {
		sph.logErr(err)
	}
}

type CreateServerMountRequest struct {
	Target   string            `json:"target"`
	Source   string            `json:"source"`
	FsType   string            `json:"fs_type"`
	FsParams fileserver.Params `json:"fs_params"`
}

func (sph *ServerPoolHandler) createServerMount(w http.ResponseWriter, r *http.Request) {
	srv := sph.serverOr404(w, r)
	if srv == nil {
		return
	}
	defer r.Body.Close()
	var req CreateServerMountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	exists, err := srv.MountMap.Put(req.Target, req.Source, req.FsType, req.FsParams)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	mount := Mount{
		ID:     mountID(req.Target),
		Source: srv.MountMap.GetSource(req.Target),
		Target: req.Target,
	}
	if exists {
		sph.logfSrv(srv, "updated mount point %s: mount %s on http://%s%s", mount.ID, mount.Source, srv.Addr, mount.Target)
	} else {
		sph.logfSrv(srv, "created mount point %s: mount %s on http://%s%s", mount.ID, mount.Source, srv.Addr, mount.Target)
	}

	sph.writeLocation(w, ServersSelfMountsSelfRoute{Port: srv.Port, ID: mount.ID})
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(mount); err != nil {
		sph.logErr(err)
	}
}

func (sph *ServerPoolHandler) removeServerMount(w http.ResponseWriter, r *http.Request) {
	srv := sph.serverOr404(w, r)
	if srv == nil {
		return
	}
	reqMountID := mux.Vars(r)["mount"]
	var mountTarget string
	for _, mt := range srv.MountMap.Targets() {
		if mountID(mt) == reqMountID {
			mountTarget = mt
			break
		}
	}
	ok := srv.MountMap.DeleteTarget(mountTarget)
	if !ok {
		http.Error(w, fmt.Sprintf("server %d has no mount target %q", srv.Port, mountTarget), http.StatusNotFound)
		return
	}
	sph.logfSrv(srv, "removed mount point %s", reqMountID)

	w.WriteHeader(http.StatusOK)
}

type FileServerTypes []string

func (sph *ServerPoolHandler) getFileServers(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(FileServerTypes(sph.ServerPool.Fsf.Types())); err != nil {
		sph.logErr(err)
	}
}
