package admin

import (
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/vincent-petithory/kraken"
	"github.com/vincent-petithory/kraken/fileserver"
)

// additional routes
const routeEvents = "events"

type RouteEvents struct{}

func (r RouteEvents) Location(rr RouteReverser) *url.URL {
	return rr.ReverseRoute(routeEvents)
}

//go:generate dispel -t all -hrt *ServerPoolHandler -d all schema.json

type ServerPoolHandler struct {
	*kraken.ServerPool
	Log    *log.Logger
	h      http.Handler
	router *GorillaRouter
	events *serverPoolEventsHandler
}

type registerHandlerFunc func(routeName string, handler http.Handler)

func (f registerHandlerFunc) RegisterHandler(routeName string, handler http.Handler) {
	f(routeName, handler)
}

func NewServerPoolRoutes(baseURL *url.URL) RouteReverser {
	router := &GorillaRouter{
		Router:  mux.NewRouter(),
		BaseURL: baseURL,
	}
	registerRoutes(router)
	return router
}

func NewServerPoolHandler(serverPool *kraken.ServerPool, baseURL *url.URL) *ServerPoolHandler {
	sph := ServerPoolHandler{
		ServerPool: serverPool,
		router: &GorillaRouter{
			Router:  mux.NewRouter(),
			BaseURL: baseURL,
		},
		events: &serverPoolEventsHandler{
			conns:   make(map[*conn]bool),
			events:  make(chan string),
			sub:     make(chan *conn),
			unsub:   make(chan *conn),
			eventCh: make(chan *Event),
		},
	}

	resHandler := func(h http.Handler) http.Handler {
		return handlers.CompressHandler(
			jsonResponseRewriteHandler(
				&sph, handlers.HTTPMethodOverrideHandler(
					handlers.ContentTypeHandler(h, "application/json"),
				),
			),
		)
	}

	jsonCodec := &JSONCodec{}
	registerRoutes(sph.router)
	registerHandlers(
		registerHandlerFunc(func(routeName string, handler http.Handler) {
			sph.router.RegisterHandler(routeName, resHandler(handler))
		}),
		sph.router,
		&sph,
		jsonCodec, jsonCodec,
		sph.endpointHandler,
	)

	sph.router.RegisterRoute("/events", routeEvents)
	sph.router.RegisterHandler(routeEvents, sph.events)

	logger := func(h http.Handler) http.Handler {
		dw := &dynamicWriter{func(b []byte) (int, error) {
			if sph.Log != nil {
				sph.Log.Printf("%s", b)
			} else {
				log.Printf("%s", b)
			}
			return len(b), nil
		}}
		return handlers.CombinedLoggingHandler(dw, h)
	}
	sph.h = logger(sph.router)

	go sph.events.Broadcast()
	return &sph
}

func (sph *ServerPoolHandler) endpointHandler(f errorHTTPHandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, err := f(w, r)
		if err != nil {
			log.Printf("HTTP Status %d: %v", status, err)
			http.Error(w, err.Error(), status)
		}
	})
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

func (sph *ServerPoolHandler) BaseURL() *url.URL {
	return &(*(sph.router.BaseURL))
}

func (sph *ServerPoolHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sph.h.ServeHTTP(w, r)
}

func newServerDataFromServer(srv *kraken.Server) *Server {
	mountTargets := srv.MountMap.Targets()
	mounts := make([]Mount, 0, len(mountTargets))
	for _, mountTarget := range mountTargets {
		mounts = append(mounts, Mount{
			Id:     mountID(mountTarget),
			Source: srv.MountMap.GetSource(mountTarget),
			Target: mountTarget,
		})
	}
	host, _, _ := net.SplitHostPort(srv.Addr)
	return &Server{
		BindAddress: host,
		Port:        int(srv.Port),
		Mounts:      mounts,
	}
}

func mountID(target string) string {
	h := sha1.New()
	h.Write([]byte(target))
	b := h.Sum(nil)
	return fmt.Sprintf("%x", b)[0:7]
}

func (sph *ServerPoolHandler) addAndStartSrv(bindAddress string, port string) (*kraken.Server, error) {
	addr := net.JoinHostPort(bindAddress, port)
	srv, err := sph.ServerPool.Add(addr)
	if err != nil {
		return nil, err
	}

	// Add middlewares to the server
	srv.HandlerWrapper = func(handler http.Handler) http.Handler {
		logger := func(h http.Handler) http.Handler {
			dw := &dynamicWriter{func(b []byte) (int, error) {
				sph.logfSrv(srv, "%s", b)
				return len(b), nil
			}}
			return handlers.CombinedLoggingHandler(dw, h)
		}
		eventsLogger := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				rsl := &responseStatusLogger{ResponseWriter: w}
				h.ServeHTTP(rsl, r)
				sph.events.Send(Event{EventTypeFileServe, FileServeEvent{*newServerDataFromServer(srv), r.URL.Path, rsl.Status}})
			})
		}
		return logger(eventsLogger(handler))
	}

	if ok := sph.ServerPool.StartSrv(srv); !ok {
		sph.logErrSrv(srv, "unable to start server")
		return nil, fmt.Errorf("unable to start server on port %d", srv.Port)
	}
	// Wait for the server to be started
	<-srv.Started
	sph.logf("created server %q", srv.Addr)
	sph.logfSrv(srv, "server available on http://%s", srv.Addr)
	sph.events.Send(Event{EventTypeServerAdd, ServerEvent{*newServerDataFromServer(srv)}})
	return srv, nil
}

type dynamicWriter struct {
	wFn func([]byte) (int, error)
}

func (dw *dynamicWriter) Write(b []byte) (int, error) {
	if dw.wFn != nil {
		return dw.wFn(b)
	}
	return ioutil.Discard.Write(b)
}

type responseStatusLogger struct {
	http.ResponseWriter
	Status int
}

func (rsl *responseStatusLogger) Write(b []byte) (int, error) {
	if rsl.Status == 0 {
		rsl.Status = http.StatusOK
	}
	return rsl.ResponseWriter.Write(b)
}

func (rsl *responseStatusLogger) WriteHeader(s int) {
	rsl.ResponseWriter.WriteHeader(s)
	rsl.Status = s
}

// Override this type from dispel

type FsParams fileserver.Params
