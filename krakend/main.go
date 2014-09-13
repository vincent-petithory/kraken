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
	"github.com/vincent-petithory/kraken/fileserver/beachplug"
)

const (
	// Environnement var for the addr of the admin service.
	// It takes precedence on the -http flag.
	envKrakenAddr = "KRAKEN_ADDR"
	// Environnement var for the base URL of the admin service.
	envKrakenURL = "KRAKEN_URL"
	// Default value of KRAKEN_ADDR
	defaultAddr = "localhost:4214"
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr,
			`Usage: krakend

krakend is an on-demand http server. It creates http servers at runtime through a RESTful API.
Those servers are meant to serve static files.

Environment vars
----------------

krakend takes no arguments and is configured through the following environment variables:

    %s: Address to bind to and port to listen to; defaults to %s
    %s: URL on which the API is accessible; defaults to http://{KRAKEN_ADDR}

See krakenctl for a command-line client of the API.
`, envKrakenAddr, defaultAddr, envKrakenURL)
	}
	flag.Parse()
}

func main() {
	log.SetOutput(os.Stdout)
	adminAddr := defaultAddr
	// Register fileservers
	fsf := make(fileserver.Factory)
	if err := fsf.Register("beachplug", beachplug.Server); err != nil {
		log.Fatal(err)
	}
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

	srv := &http.Server{
		Handler: sph,
	}
	log.Printf("Listening on %s", ln.Addr())
	log.Printf("Available on %s", sph.BaseURL())
	log.Fatal(srv.Serve(ln))
}
