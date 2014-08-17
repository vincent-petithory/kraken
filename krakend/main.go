package main

import (
	"flag"
	"log"
	"net"
	"net/http"

	"github.com/vincent-petithory/kraken"
	"github.com/vincent-petithory/kraken/fileserver"
)

var adminAddr string

func init() {
	flag.StringVar(&adminAddr, "http", ":4214", "The addr on which the admin http api will listen on. Defaults to :4214")
	flag.Parse()
}

func main() {
	// Register fileservers
	fsf := make(fileserver.Factory)

	// Init server pool, run existing servers and listen for new ones
	serverPool := kraken.NewServerPool(fsf)
	go serverPool.ListenAndRun()

	// Start administration server
	spah := kraken.NewServerPoolAdminHandler(serverPool)

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
