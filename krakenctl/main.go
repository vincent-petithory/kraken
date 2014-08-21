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
		Use:   "add [PORT]",
		Short: "Add a new server",
		Long:  "Add a new server listening on PORT, or a random port if not provided",
		Run:   clientCmd(client, flags, serverAdd),
	}
	serverAddCmd.Flags().StringVarP(&flags.ServerAddBind, "bind", "b", "", "Address to bind to, defaults to not bind.")

	serverRmCmd := &cobra.Command{
		Use:   "rm PORT",
		Short: "Removes a server",
		Long:  "Removes a server listening on PORT",
		Run:   clientCmd(client, flags, serverRm),
	}

	serverClearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Removes all servers",
		Long:  "Removes all available servers",
		Run:   clientCmd(client, flags, serverRmAll),
	}

	rootCmd := &cobra.Command{
		Use: "krakenctl",
	}
	rootCmd.AddCommand(
		serverAddCmd,
		serverRmCmd,
		serverClearCmd,
	)
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

func serverRm(client *client, flags *flagSet, cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		cmd.Usage()
		return
	}
	if len(args) > 1 {
		cmd.Usage()
		return
	}
	port, err := strconv.Atoi(args[0])
	if err != nil {
		log.Fatalf("error parsing port: %v", err)
	}
	if err := client.RemoveServer(uint16(port)); err != nil {
		log.Fatal(err)
	}
}

func serverRmAll(client *client, flags *flagSet, cmd *cobra.Command, args []string) {
	if len(args) > 0 {
		cmd.Usage()
		return
	}
	if err := client.RemoveAllServers(); err != nil {
		log.Fatal(err)
	}
}
