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

type serverPoolHandler struct {
	*kraken.ServerPool
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
	URL(*ServerPoolRoutes) *url.URL
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

func (r ServersRoute) URL(spr *ServerPoolRoutes) *url.URL {
	return spr.url(routeServers)
}
func (r ServersSelfRoute) URL(spr *ServerPoolRoutes) *url.URL {
	return spr.url(routeServersSelf, "port", strconv.Itoa(int(r.Port)))
}
func (r ServersSelfMountsRoute) URL(spr *ServerPoolRoutes) *url.URL {
	return spr.url(routeServersSelfMounts, "port", strconv.Itoa(int(r.Port)))
}
func (r ServersSelfMountsSelfRoute) URL(spr *ServerPoolRoutes) *url.URL {
	return spr.url(routeServersSelfMountsSelf, "port", strconv.Itoa(int(r.Port)), "mount", r.ID)
}
func (r FileServersRoute) URL(spr *ServerPoolRoutes) *url.URL {
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

func (spr *ServerPoolRoutes) url(name string, params ...string) *url.URL {
	var urlPath string
	if u, err := spr.r.Get(name).URLPath(params...); err != nil {
		log.Print(err)
		urlPath = ""
	} else {
		urlPath = u.Path
	}
	var u url.URL
	if spr.BaseURL != nil {
		u = (*spr.BaseURL)
	}
	u.Path = urlPath
	return &u
}

func (spr *ServerPoolRoutes) RouteURL(r Route) *url.URL {
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

func NewServerPoolHandler(serverPool *kraken.ServerPool) *serverPoolHandler {
	sph := serverPoolHandler{
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

func (sph *serverPoolHandler) SetBaseURL(u *url.URL) {
	sph.routes.BaseURL = u
}

func (sph *serverPoolHandler) BaseURL() *url.URL {
	return &(*(sph.routes.BaseURL))
}

func (sph *serverPoolHandler) writeLocation(w http.ResponseWriter, route Route) {
	w.Header().Set("Location", sph.routes.RouteURL(route).String())
}

func (sph *serverPoolHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func (sph *serverPoolHandler) serverOr404(w http.ResponseWriter, r *http.Request) *kraken.Server {
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

func (sph *serverPoolHandler) getServers(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	srvs := make([]Server, 0, len(sph.ServerPool.Srvs))
	for _, srv := range sph.ServerPool.Srvs {
		srvs = append(srvs, *newServerDataFromServer(srv))
	}
	if err := json.NewEncoder(w).Encode(srvs); err != nil {
		log.Print(err)
	}
}

func (sph *serverPoolHandler) getServer(w http.ResponseWriter, r *http.Request) {
	srv := sph.serverOr404(w, r)
	if srv == nil {
		return
	}
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(newServerDataFromServer(srv)); err != nil {
		log.Print(err)
	}
}

type CreateServerRequest struct {
	BindAddress string `json:"bind_address"`
}

func (sph *serverPoolHandler) createServerWithRandomPort(w http.ResponseWriter, r *http.Request) {
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
	sph.writeLocation(w, ServersSelfRoute{srv.Port})
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(newServerDataFromServer(srv)); err != nil {
		log.Print(err)
	}
}

func (sph *serverPoolHandler) createServer(w http.ResponseWriter, r *http.Request) {
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
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(newServerDataFromServer(srv)); err != nil {
		log.Print(err)
	}
}

func (sph *serverPoolHandler) removeServers(w http.ResponseWriter, r *http.Request) {
	var errs []error
	srvs := make([]*kraken.Server, len(sph.ServerPool.Srvs))
	copy(srvs, sph.ServerPool.Srvs)
	for _, srv := range srvs {
		if ok, err := sph.ServerPool.Remove(srv.Port); err != nil {
			errs = append(errs, err)
		} else if !ok {
			err := fmt.Errorf("error shutting down server on port %d", srv.Port)
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		var bufMsg bytes.Buffer
		for _, err := range errs {
			fmt.Fprintln(&bufMsg, err.Error())
		}
		apiErr := &APIError{apiErrTypeAPIInternal, bufMsg.String()}
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(apiErr); err != nil {
			log.Print(err)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (sph *serverPoolHandler) removeServer(w http.ResponseWriter, r *http.Request) {
	srv := sph.serverOr404(w, r)
	if srv == nil {
		return
	}
	if ok, err := sph.ServerPool.Remove(srv.Port); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if !ok {
		err := fmt.Errorf("error shutting down server on port %d", srv.Port)
		log.Print(err)
	}
	w.WriteHeader(http.StatusOK)
}

func (sph *serverPoolHandler) getServerMounts(w http.ResponseWriter, r *http.Request) {
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
		log.Print(err)
	}
}

func (sph *serverPoolHandler) removeServerMounts(w http.ResponseWriter, r *http.Request) {
	srv := sph.serverOr404(w, r)
	if srv == nil {
		return
	}
	mountTargets := srv.MountMap.Targets()
	for _, mountTarget := range mountTargets {
		srv.MountMap.DeleteTarget(mountTarget)
	}
	w.WriteHeader(http.StatusOK)
}

func (sph *serverPoolHandler) getServerMount(w http.ResponseWriter, r *http.Request) {
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
		log.Print(err)
	}
}

type CreateServerMountRequest struct {
	Target   string            `json:"target"`
	Source   string            `json:"source"`
	FsType   string            `json:"fs_type"`
	FsParams fileserver.Params `json:"fs_params"`
}

func (sph *serverPoolHandler) createServerMount(w http.ResponseWriter, r *http.Request) {
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
	if _, err := srv.MountMap.Put(req.Target, req.Source, req.FsType, req.FsParams); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	mount := Mount{
		ID:     mountID(req.Target),
		Source: srv.MountMap.GetSource(req.Target),
		Target: req.Target,
	}
	sph.writeLocation(w, ServersSelfMountsSelfRoute{Port: srv.Port, ID: mount.ID})
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(mount); err != nil {
		log.Print(err)
	}
}

func (sph *serverPoolHandler) removeServerMount(w http.ResponseWriter, r *http.Request) {
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
	w.WriteHeader(http.StatusOK)
}

type FileServerTypes []string

func (sph *serverPoolHandler) getFileServers(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(FileServerTypes(sph.ServerPool.Fsf.Types())); err != nil {
		log.Print(err)
	}
}
