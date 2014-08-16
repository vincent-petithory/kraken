package main

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
)

type serverPoolAdminHandler struct {
	*serverPool
	h      http.Handler
	router *mux.Router
}

const (
	LocationServers                = "servers"
	LocationServersSelf            = "servers.self"
	LocationServersSelfAliases     = "servers.self.aliases"
	LocationServersSelfAliasesSelf = "servers.self.aliases.self"
)

type adminApiErrorType string

const (
	ApiErrTypeBadRequest  adminApiErrorType = "bad_request_error"
	ApiErrTypeApiInternal                   = "api_internal_error"
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

func NewServerPoolAdminHandler(serverPool *serverPool) *serverPoolAdminHandler {
	spah := serverPoolAdminHandler{serverPool: serverPool}
	r := mux.NewRouter()
	apiRouter := r.PathPrefix("/api/").Subrouter()
	apiRouter.Handle("/servers", handlers.MethodHandler{
		"GET":    http.HandlerFunc(spah.getServers),
		"POST":   http.HandlerFunc(spah.createServerWithRandomPort),
		"DELETE": http.HandlerFunc(spah.removeServers),
	}).Name(LocationServers)
	apiRouter.Handle("/servers/{port:[0-9]{1,5}}", handlers.MethodHandler{
		"GET":    http.HandlerFunc(spah.getServer),
		"PUT":    http.HandlerFunc(spah.createServer),
		"DELETE": http.HandlerFunc(spah.removeServer),
	}).Name(LocationServersSelf)
	apiRouter.Handle("/servers/{port:[0-9]{1,5}}/aliases", handlers.MethodHandler{
		"GET":    http.HandlerFunc(spah.getServerAliases),
		"DELETE": http.HandlerFunc(spah.removeServerAliases),
	}).Name(LocationServersSelfAliases)
	apiRouter.Handle("/servers/{port:[0-9]{1,5}}/aliases/{alias}", handlers.MethodHandler{
		"GET":    http.HandlerFunc(spah.getServerAlias),
		"PUT":    http.HandlerFunc(spah.createServerAlias),
		"DELETE": http.HandlerFunc(spah.removeServerAlias),
	}).Name(LocationServersSelfAliasesSelf)
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

type dirServerData struct {
	BindAddress string            `json:"bind_address"`
	Port        uint16            `json:"port"`
	Aliases     map[string]string `json:"aliases"`
}

func dirServerDataFromDirServer(ds *dirServer) dirServerData {
	aliases := ds.DirAliases.List()
	aliasesMap := make(map[string]string, len(aliases))
	for _, alias := range aliases {
		aliasesMap[alias] = ds.DirAliases.Get(alias)
	}
	host, _, _ := net.SplitHostPort(ds.Addr)
	return dirServerData{
		BindAddress: host,
		Port:        ds.Port,
		Aliases:     aliasesMap,
	}
}

func (spah *serverPoolAdminHandler) serverOr404(w http.ResponseWriter, r *http.Request) *dirServer {
	sport := mux.Vars(r)["port"]
	port, err := strconv.Atoi(sport)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}
	srv := spah.serverPool.Get(uint16(port))
	if srv == nil {
		http.Error(w, fmt.Sprintf("server %d not found", port), http.StatusNotFound)
		return nil
	}
	return srv
}

func (spah *serverPoolAdminHandler) getServers(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	srvsData := make([]dirServerData, 0, len(spah.serverPool.Srvs))
	for _, srv := range spah.serverPool.Srvs {
		srvsData = append(srvsData, dirServerDataFromDirServer(srv))
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
	if err := json.NewEncoder(w).Encode(dirServerDataFromDirServer(srv)); err != nil {
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
	srv, err := spah.serverPool.Add(addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Wait for the server to be started
	<-srv.started
	spah.writeLocation(w, LocationServersSelf, "port", strconv.Itoa(int(srv.Port)))
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(dirServerDataFromDirServer(srv)); err != nil {
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
	srv, err := spah.serverPool.Add(addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Wait for the server to be started
	<-srv.started
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(dirServerDataFromDirServer(srv)); err != nil {
		log.Print(err)
	}
}

func (spah *serverPoolAdminHandler) removeServers(w http.ResponseWriter, r *http.Request) {
	errs := make([]error, 0)
	for _, srv := range spah.serverPool.Srvs {
		if ok, err := spah.serverPool.Remove(srv.Port); err != nil {
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
		apiErr := &adminApiError{ApiErrTypeApiInternal, bufMsg.String()}
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
	if ok, err := spah.serverPool.Remove(srv.Port); err != nil {
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
	Path string `json:"path"`
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
	_ = srv.DirAliases.Put(alias, req.Path)
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
