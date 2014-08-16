package main

import (
	"flag"
	"log"
	"net"
	"net/http"
)

var adminAddr string

func init() {
	flag.StringVar(&adminAddr, "http", ":4214", "The addr on which the admin http api will listen on. Defaults to :4214")
	flag.Parse()
}

func main() {
	// Init server pool, run existing servers and listen for new ones
	serverPool := &serverPool{
		Srvs:  make([]*dirServer, 0),
		SrvCh: make(chan *dirServer),
	}
	go serverPool.ListenAndRun()

	// Start administration server
	spah := NewServerPoolAdminHandler(serverPool)

	ln, err := net.Listen("tcp", adminAddr)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{
		Handler: spah,
	}
	log.Printf("[admin] Listening on %s", ln.Addr())
	log.Fatal(srv.Serve(ln))
}
