package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

// Environnement var for the url on which the admin service is accessible.
const envKrakenURL = "KRAKEN_URL"

func loadKrakenURL() (*url.URL, error) {
	rawurl := os.Getenv(envKrakenURL)
	if rawurl == "" {
		rawurl = "http://localhost:4214"
	}
	u, err := url.Parse(rawurl)
	if err != nil {
		return nil, err
	}
	if !u.IsAbs() {
		return nil, fmt.Errorf("%v is not an absolute URL", u)
	}
	if u.Path != "" {
		return nil, fmt.Errorf("%v has a path, which is not allowed", u)
	}
	return u, nil
}

type flagSet struct {
	ServerAddBind string
}

func clientCmd(client *client, flags *flagSet, runFn func(*client, *flagSet, *cobra.Command, []string)) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, args []string) {
		runFn(client, flags, cmd, args)
	}
}

func main() {
	krakenURL, err := loadKrakenURL()
	if err != nil {
		log.Fatal(err)
	}
	client := &client{
		c:   &http.Client{},
		url: krakenURL,
	}

	flags := &flagSet{}
	serverAddCmd := &cobra.Command{
		Use:   "add [port]",
		Short: "Add a new server",
		Long:  "Add a new server listening on [port], or a random port if not provided",
		Run:   clientCmd(client, flags, serverAdd),
	}
	serverAddCmd.Flags().StringVarP(&flags.ServerAddBind, "bind", "b", "", "Address to bind to, defaults to not bind.")

	rootCmd := &cobra.Command{
		Use: "krakenctl",
	}
	rootCmd.AddCommand(serverAddCmd)
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func serverAdd(client *client, flags *flagSet, cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		if err := client.AddServerWithRandomPort(flags.ServerAddBind); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(args) > 1 {
		cmd.Usage()
		fmt.Fprintln(os.Stderr, "too many args provided")
		return
	}
	port, err := strconv.Atoi(args[0])
	if err != nil {
		log.Fatalf("error parsing port: %v", err)
	}
	if err := client.AddServer(flags.ServerAddBind, uint16(port)); err != nil {
		log.Fatal(err)
	}
}
