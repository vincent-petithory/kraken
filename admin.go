package kraken

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"log"
	"net"
	"net/http"
	"strconv"

	"github.com/vincent-petithory/kraken/fileserver"
)

type serverPoolAdminHandler struct {
	*ServerPool
	h      http.Handler
	router *mux.Router
}

const (
	routeServers                = "servers"
	routeServersSelf            = "servers.self"
	routeServersSelfAliases     = "servers.self.aliases"
	routeServersSelfAliasesSelf = "servers.self.aliases.self"
	routeFileservers            = "fileservers"
)

type adminApiErrorType string

const (
	apiErrTypeBadRequest  adminApiErrorType = "bad_request_error"
	apiErrTypeApiInternal                   = "api_internal_error"
)

type adminApiError struct {
	Type adminApiErrorType `json:"type"`
	Msg  string            `json:"msg"`
}

func (e *adminApiError) String() string {
	return fmt.Sprintf("%s: %s", e.Type, e.Msg)
}

func (e *adminApiError) Error() string {
	return e.Msg
}

func NewServerPoolAdminHandler(serverPool *ServerPool) *serverPoolAdminHandler {
	spah := serverPoolAdminHandler{ServerPool: serverPool}
	r := mux.NewRouter()
	apiRouter := r.PathPrefix("/api/").Subrouter()
	apiRouter.Handle("/servers", handlers.MethodHandler{
		"GET":    http.HandlerFunc(spah.getServers),
		"POST":   http.HandlerFunc(spah.createServerWithRandomPort),
		"DELETE": http.HandlerFunc(spah.removeServers),
	}).Name(routeServers)
	apiRouter.Handle("/servers/{port:[0-9]{1,5}}", handlers.MethodHandler{
		"GET":    http.HandlerFunc(spah.getServer),
		"PUT":    http.HandlerFunc(spah.createServer),
		"DELETE": http.HandlerFunc(spah.removeServer),
	}).Name(routeServersSelf)
	apiRouter.Handle("/servers/{port:[0-9]{1,5}}/aliases", handlers.MethodHandler{
		"GET":    http.HandlerFunc(spah.getServerAliases),
		"DELETE": http.HandlerFunc(spah.removeServerAliases),
	}).Name(routeServersSelfAliases)
	apiRouter.Handle("/servers/{port:[0-9]{1,5}}/aliases/{alias}", handlers.MethodHandler{
		"GET":    http.HandlerFunc(spah.getServerAlias),
		"PUT":    http.HandlerFunc(spah.createServerAlias),
		"DELETE": http.HandlerFunc(spah.removeServerAlias),
	}).Name(routeServersSelfAliasesSelf)
	apiRouter.Handle("/fileservers", handlers.MethodHandler{
		"GET": http.HandlerFunc(spah.getFileServers),
	}).Name(routeFileservers)
	spah.h = r
	spah.router = r
	return &spah
}

func (spah *serverPoolAdminHandler) writeLocation(w http.ResponseWriter, name string, params ...string) {
	var urlStr string
	if u, err := spah.router.GetRoute(name).URL(params...); err != nil {
		log.Print(err)
		urlStr = ""
	} else {
		urlStr = u.String()
	}
	w.Header().Set("Location", urlStr)
}

func (spah *serverPoolAdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	spah.h.ServeHTTP(w, r)
}

type serverData struct {
	BindAddress string            `json:"bind_address"`
	Port        uint16            `json:"port"`
	Aliases     map[string]string `json:"aliases"`
}

func newServerDataFromServer(ds *Server) *serverData {
	aliases := ds.DirAliases.List()
	aliasesMap := make(map[string]string, len(aliases))
	for _, alias := range aliases {
		aliasesMap[alias] = ds.DirAliases.Get(alias)
	}
	host, _, _ := net.SplitHostPort(ds.Addr)
	return &serverData{
		BindAddress: host,
		Port:        ds.Port,
		Aliases:     aliasesMap,
	}
}

func (spah *serverPoolAdminHandler) serverOr404(w http.ResponseWriter, r *http.Request) *Server {
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
	srvsData := make([]serverData, 0, len(spah.ServerPool.Srvs))
	for _, srv := range spah.ServerPool.Srvs {
		srvsData = append(srvsData, *newServerDataFromServer(srv))
	}
	if err := json.NewEncoder(w).Encode(srvsData); err != nil {
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

type createServerRequest struct {
	BindAddress string `json:"bind_address"`
}

func (spah *serverPoolAdminHandler) createServerWithRandomPort(w http.ResponseWriter, r *http.Request) {
	var req createServerRequest
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
	<-srv.started
	spah.writeLocation(w, routeServersSelf, "port", strconv.Itoa(int(srv.Port)))
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(newServerDataFromServer(srv)); err != nil {
		log.Print(err)
	}
}

func (spah *serverPoolAdminHandler) createServer(w http.ResponseWriter, r *http.Request) {
	var req createServerRequest
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
	<-srv.started
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(newServerDataFromServer(srv)); err != nil {
		log.Print(err)
	}
}

func (spah *serverPoolAdminHandler) removeServers(w http.ResponseWriter, r *http.Request) {
	errs := make([]error, 0)
	for _, srv := range spah.ServerPool.Srvs {
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
		apiErr := &adminApiError{apiErrTypeApiInternal, bufMsg.String()}
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

func (spah *serverPoolAdminHandler) getServerAliases(w http.ResponseWriter, r *http.Request) {
	srv := spah.serverOr404(w, r)
	if srv == nil {
		return
	}
	aliases := srv.DirAliases.List()
	aliasesMap := make(map[string]string, len(aliases))

	for _, alias := range aliases {
		aliasesMap[alias] = srv.DirAliases.Get(alias)
	}
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(aliasesMap); err != nil {
		log.Print(err)
	}
}

func (spah *serverPoolAdminHandler) removeServerAliases(w http.ResponseWriter, r *http.Request) {
	srv := spah.serverOr404(w, r)
	if srv == nil {
		return
	}
	aliases := srv.DirAliases.List()
	for _, alias := range aliases {
		srv.DirAliases.Delete(alias)
	}
	w.WriteHeader(http.StatusOK)
}

func (spah *serverPoolAdminHandler) getServerAlias(w http.ResponseWriter, r *http.Request) {
	srv := spah.serverOr404(w, r)
	if srv == nil {
		return
	}
	alias := mux.Vars(r)["alias"]
	aliasPath := srv.DirAliases.Get(alias)
	if aliasPath == "" {
		http.Error(w, fmt.Sprintf("server %d has no alias %q", srv.Port, alias), http.StatusNotFound)
		return
	}
	aliasesMap := map[string]string{alias: aliasPath}
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(aliasesMap); err != nil {
		log.Print(err)
	}
}

type createServerAliasRequest struct {
	Path     string            `json:"path"`
	FsType   string            `json:"fs_type"`
	FsParams fileserver.Params `json:"fs_params"`
}

func (spah *serverPoolAdminHandler) createServerAlias(w http.ResponseWriter, r *http.Request) {
	srv := spah.serverOr404(w, r)
	if srv == nil {
		return
	}
	alias := mux.Vars(r)["alias"]
	defer r.Body.Close()
	var req createServerAliasRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = srv.DirAliases.Put(alias, req.Path, req.FsType, req.FsParams)
	w.WriteHeader(http.StatusOK)
}

func (spah *serverPoolAdminHandler) removeServerAlias(w http.ResponseWriter, r *http.Request) {
	srv := spah.serverOr404(w, r)
	if srv == nil {
		return
	}
	alias := mux.Vars(r)["alias"]
	ok := srv.DirAliases.Delete(alias)
	if !ok {
		http.Error(w, fmt.Sprintf("server %d has no alias %q", srv.Port, alias), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (spah *serverPoolAdminHandler) getFileServers(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(spah.ServerPool.fsf.Types()); err != nil {
		log.Print(err)
	}
}
