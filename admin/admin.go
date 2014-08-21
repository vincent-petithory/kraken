package admin

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/vincent-petithory/kraken"
	"github.com/vincent-petithory/kraken/fileserver"
)

type serverPoolAdminHandler struct {
	*kraken.ServerPool
	h      http.Handler
	routes *ServerPoolAdminRoutes
}

const (
	routeServers               = "servers"
	routeServersSelf           = "servers.self"
	routeServersSelfMounts     = "servers.self.mounts"
	routeServersSelfMountsSelf = "servers.self.mounts.self"
	routeFileservers           = "fileservers"
)

type Route interface {
	URL(*ServerPoolAdminRoutes) *url.URL
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
)

func (r ServersRoute) URL(spar *ServerPoolAdminRoutes) *url.URL {
	return spar.url(routeServers)
}

func (r ServersSelfRoute) URL(spar *ServerPoolAdminRoutes) *url.URL {
	return spar.url(routeServersSelf, "port", strconv.Itoa(int(r.Port)))
}
func (r ServersSelfMountsRoute) URL(spar *ServerPoolAdminRoutes) *url.URL {
	return spar.url(routeServersSelfMounts, "port", strconv.Itoa(int(r.Port)))
}
func (r ServersSelfMountsSelfRoute) URL(spar *ServerPoolAdminRoutes) *url.URL {
	return spar.url(routeServersSelfMountsSelf, "port", strconv.Itoa(int(r.Port)), "mount", r.ID)
}

type AdminAPIErrorType string

const (
	apiErrTypeBadRequest  AdminAPIErrorType = "bad_request_error"
	apiErrTypeAPIInternal                   = "api_internal_error"
)

type AdminAPIError struct {
	Type AdminAPIErrorType `json:"type"`
	Msg  string            `json:"msg"`
}

func (e *AdminAPIError) String() string {
	return fmt.Sprintf("%s: %s", e.Type, e.Msg)
}

func (e *AdminAPIError) Error() string {
	return e.Msg
}

type ServerPoolAdminRoutes struct {
	r       *mux.Router
	BaseURL *url.URL
}

func (spar *ServerPoolAdminRoutes) url(name string, params ...string) *url.URL {
	var urlPath string
	if u, err := spar.r.Get(name).URLPath(params...); err != nil {
		log.Print(err)
		urlPath = ""
	} else {
		urlPath = u.Path
	}
	var u url.URL
	if spar.BaseURL != nil {
		u = (*spar.BaseURL)
	}
	u.Path = urlPath
	return &u
}

func (spar *ServerPoolAdminRoutes) RouteURL(r Route) *url.URL {
	return r.URL(spar)
}

func NewServerPoolAdminRoutes() *ServerPoolAdminRoutes {
	r := mux.NewRouter()
	apiRouter := r.PathPrefix("/api/").Subrouter()
	apiRouter.Path("/servers").Name(routeServers)
	apiRouter.Path("/servers/{port:[0-9]{1,5}}").Name(routeServersSelf)
	apiRouter.Path("/servers/{port:[0-9]{1,5}}/mounts").Name(routeServersSelfMounts)
	apiRouter.Path("/servers/{port:[0-9]{1,5}}/mounts/{mount}").Name(routeServersSelfMountsSelf)
	apiRouter.Path("/fileservers").Name(routeFileservers)
	return &ServerPoolAdminRoutes{r: r}
}

func NewServerPoolAdminHandler(serverPool *kraken.ServerPool) *serverPoolAdminHandler {
	spah := serverPoolAdminHandler{
		ServerPool: serverPool,
		routes:     NewServerPoolAdminRoutes(),
	}
	spah.routes.r.Get(routeServers).Handler(handlers.MethodHandler{
		"GET":    http.HandlerFunc(spah.getServers),
		"POST":   http.HandlerFunc(spah.createServerWithRandomPort),
		"DELETE": http.HandlerFunc(spah.removeServers),
	})
	spah.routes.r.Get(routeServersSelf).Handler(handlers.MethodHandler{
		"GET":    http.HandlerFunc(spah.getServer),
		"PUT":    http.HandlerFunc(spah.createServer),
		"DELETE": http.HandlerFunc(spah.removeServer),
	})
	spah.routes.r.Get(routeServersSelfMounts).Handler(handlers.MethodHandler{
		"GET":    http.HandlerFunc(spah.getServerMounts),
		"POST":   http.HandlerFunc(spah.createServerMount),
		"DELETE": http.HandlerFunc(spah.removeServerMounts),
	})
	spah.routes.r.Get(routeServersSelfMountsSelf).Handler(handlers.MethodHandler{
		"GET":    http.HandlerFunc(spah.getServerMount),
		"DELETE": http.HandlerFunc(spah.removeServerMount),
	})
	spah.routes.r.Get(routeFileservers).Handler(handlers.MethodHandler{
		"GET": http.HandlerFunc(spah.getFileServers),
	})
	spah.h = spah.routes.r
	return &spah
}

func (spah *serverPoolAdminHandler) SetBaseURL(u *url.URL) {
	spah.routes.BaseURL = u
}

func (spah *serverPoolAdminHandler) BaseURL() *url.URL {
	return &(*(spah.routes.BaseURL))
}

func (spah *serverPoolAdminHandler) writeLocation(w http.ResponseWriter, route Route) {
	w.Header().Set("Location", spah.routes.RouteURL(route).String())
}

func (spah *serverPoolAdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	spah.h.ServeHTTP(w, r)
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

func (spah *serverPoolAdminHandler) serverOr404(w http.ResponseWriter, r *http.Request) *kraken.Server {
	sport := mux.Vars(r)["port"]
	port, err := strconv.Atoi(sport)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}
	srv := spah.ServerPool.Get(uint16(port))
	if srv == nil {
		http.Error(w, fmt.Sprintf("server %d not found", port), http.StatusNotFound)
		return nil
	}
	return srv
}

func (spah *serverPoolAdminHandler) getServers(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	srvs := make([]Server, 0, len(spah.ServerPool.Srvs))
	for _, srv := range spah.ServerPool.Srvs {
		srvs = append(srvs, *newServerDataFromServer(srv))
	}
	if err := json.NewEncoder(w).Encode(srvs); err != nil {
		log.Print(err)
	}
}

func (spah *serverPoolAdminHandler) getServer(w http.ResponseWriter, r *http.Request) {
	srv := spah.serverOr404(w, r)
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

func (spah *serverPoolAdminHandler) createServerWithRandomPort(w http.ResponseWriter, r *http.Request) {
	var req CreateServerRequest
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	addr := net.JoinHostPort(req.BindAddress, "0")
	srv, err := spah.ServerPool.Add(addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Wait for the server to be started
	<-srv.Started
	spah.writeLocation(w, ServersSelfRoute{srv.Port})
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(newServerDataFromServer(srv)); err != nil {
		log.Print(err)
	}
}

func (spah *serverPoolAdminHandler) createServer(w http.ResponseWriter, r *http.Request) {
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
	srv, err := spah.ServerPool.Add(addr)
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

func (spah *serverPoolAdminHandler) removeServers(w http.ResponseWriter, r *http.Request) {
	var errs []error
	srvs := make([]*kraken.Server, len(spah.ServerPool.Srvs))
	copy(srvs, spah.ServerPool.Srvs)
	for _, srv := range srvs {
		if ok, err := spah.ServerPool.Remove(srv.Port); err != nil {
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
		apiErr := &AdminAPIError{apiErrTypeAPIInternal, bufMsg.String()}
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(apiErr); err != nil {
			log.Print(err)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (spah *serverPoolAdminHandler) removeServer(w http.ResponseWriter, r *http.Request) {
	srv := spah.serverOr404(w, r)
	if srv == nil {
		return
	}
	if ok, err := spah.ServerPool.Remove(srv.Port); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if !ok {
		err := fmt.Errorf("error shutting down server on port %d", srv.Port)
		log.Print(err)
	}
	w.WriteHeader(http.StatusOK)
}

func (spah *serverPoolAdminHandler) getServerMounts(w http.ResponseWriter, r *http.Request) {
	srv := spah.serverOr404(w, r)
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

func (spah *serverPoolAdminHandler) removeServerMounts(w http.ResponseWriter, r *http.Request) {
	srv := spah.serverOr404(w, r)
	if srv == nil {
		return
	}
	mountTargets := srv.MountMap.Targets()
	for _, mountTarget := range mountTargets {
		srv.MountMap.DeleteTarget(mountTarget)
	}
	w.WriteHeader(http.StatusOK)
}

func (spah *serverPoolAdminHandler) getServerMount(w http.ResponseWriter, r *http.Request) {
	srv := spah.serverOr404(w, r)
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

func (spah *serverPoolAdminHandler) createServerMount(w http.ResponseWriter, r *http.Request) {
	srv := spah.serverOr404(w, r)
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
	spah.writeLocation(w, ServersSelfMountsSelfRoute{Port: srv.Port, ID: mount.ID})
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(mount); err != nil {
		log.Print(err)
	}
}

func (spah *serverPoolAdminHandler) removeServerMount(w http.ResponseWriter, r *http.Request) {
	srv := spah.serverOr404(w, r)
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

func (spah *serverPoolAdminHandler) getFileServers(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(FileServerTypes(spah.ServerPool.Fsf.Types())); err != nil {
		log.Print(err)
	}
}
