package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"

	"github.com/vincent-petithory/kraken"
	"github.com/vincent-petithory/kraken/admin"
	"github.com/vincent-petithory/kraken/fileserver"
)

var adminAddr string

const (
	// Environnement var for the addr of the admin service.
	// It takes precedence on the -http flag.
	envKrakenAddr = "KRAKEN_ADDR"
	// Environnement var for the base URL of the admin service.
	envKrakenURL = "KRAKEN_URL"
)

func init() {
	flag.StringVar(&adminAddr, "http", "localhost:4214", "The address on which the admin http api will listen on. Defaults to :4214")
	flag.Parse()
}

func main() {
	// Register fileservers
	fsf := make(fileserver.Factory)

	// Init server pool, run existing servers and listen for new ones
	serverPool := kraken.NewServerPool(fsf)
	go serverPool.ListenAndRun()

	// Start administration server
	sph := admin.NewServerPoolHandler(serverPool)

	if envAdminAddr := os.Getenv(envKrakenAddr); envAdminAddr != "" {
		adminAddr = envAdminAddr
	}
	ln, err := net.Listen("tcp", adminAddr)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{
		Handler: sph,
	}

	var (
		adminURL *url.URL
		urlErr   error
	)
	adminURL, urlErr = url.Parse(fmt.Sprintf("http://%s", ln.Addr()))
	if envAdminURL := os.Getenv(envKrakenURL); envAdminURL != "" {
		adminURL, urlErr = url.Parse(envAdminURL)
	}
	if urlErr != nil {
		log.Fatal(urlErr)
	}
	sph.SetBaseURL(adminURL)

	log.Printf("[admin] Listening on %s", ln.Addr())
	log.Printf("[admin] Available on %s", sph.BaseURL())
	log.Fatal(srv.Serve(ln))
}
