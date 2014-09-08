package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincent-petithory/kraken/admin"
	"github.com/vincent-petithory/kraken/admin/client"
	"github.com/vincent-petithory/kraken/fileserver"
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
	ServerAddBind    string
	MountTarget      string
	FileServerType   string
	FileServerParams string
}

func clientCmd(client *client.Client, flags *flagSet, runFn func(*client.Client, *flagSet, *cobra.Command, []string)) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, args []string) {
		runFn(client, flags, cmd, args)
	}
}

func main() {
	log.SetFlags(0)
	krakenURL, err := loadKrakenURL()
	if err != nil {
		log.Fatal(err)
	}
	c := client.New(krakenURL)

	flags := &flagSet{}

	serversGetCmd := &cobra.Command{
		Use:   "ls",
		Short: "List the available servers",
		Long:  "List the available servers",
		Run:   clientCmd(c, flags, serverList),
	}

	serverAddCmd := &cobra.Command{
		Use:   "add [PORT]",
		Short: "Add a new server",
		Long:  "Add a new server listening on PORT, or a random port if not provided",
		Run:   clientCmd(c, flags, serverAdd),
	}
	serverAddCmd.Flags().StringVarP(&flags.ServerAddBind, "bind", "b", "", "Address to bind to, defaults to not bind")

	serverRmCmd := &cobra.Command{
		Use:   "rm PORT",
		Short: "Remove a server",
		Long:  "Remove a server listening on PORT",
		Run:   clientCmd(c, flags, serverRm),
	}

	serverClearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Remove all servers",
		Long:  "Remove all available servers",
		Run:   clientCmd(c, flags, serverRmAll),
	}

	mountsGetCmd := &cobra.Command{
		Use:   "lsmount PORT",
		Short: "List the mounts of a server",
		Long:  "List the mounts of a the server listening on PORT",
		Run:   clientCmd(c, flags, mountList),
	}

	mountAddCmd := &cobra.Command{
		Use:   "mount PORT SOURCE",
		Short: "Mount a directory on a server",
		Long: `Mount the SOURCE directory on the server listening on PORT.
By default, SOURCE is mounted on /$(basename SOURCE)`,
		Run: clientCmd(c, flags, mountAdd),
	}
	mountAddCmd.Flags().StringVarP(&flags.MountTarget, "target", "t", "", "Alternate mount target; it must start with / and not end with /")
	mountAddCmd.Flags().StringVarP(&flags.FileServerType, "fs", "f", "beachplug", "File server type to use for this mount point; if empty, a fallback is used (net/http.FileServer)")
	mountAddCmd.Flags().StringVarP(&flags.FileServerParams, "fsp", "p", "{}", "File server params; they must be specified as a valid JSON object.")

	mountRmCmd := &cobra.Command{
		Use:   "umount PORT MOUNT_ID",
		Short: "Unmount a directory on a server",
		Long:  "Removes the mount point MOUNT_ID, on the server listening on PORT",
		Run:   clientCmd(c, flags, mountRm),
	}

	fileServersGetCmd := &cobra.Command{
		Use:   "fileservers",
		Short: "Lists the available file servers",
		Long:  "Lists the available file servers",
		Run:   clientCmd(c, flags, fileServerList),
	}

	eventsCmd := &cobra.Command{
		Use:   "events [EVENT]...",
		Short: "Listen for events from kraken",
		Long:  "Listen for the specified events from kraken. If no event is provided, all events are listened for. Otherwise, only the specified events will be listened for. EVENTs can be server or mount",
		Run:   clientCmd(c, flags, listenEvents),
	}

	rootCmd := &cobra.Command{
		Use: "krakenctl",
	}
	rootCmd.AddCommand(
		// server commands
		serversGetCmd,
		serverAddCmd,
		serverRmCmd,
		serverClearCmd,
		// mount commands
		mountsGetCmd,
		mountAddCmd,
		mountRmCmd,
		// fileserver commands
		fileServersGetCmd,
		// events
		eventsCmd,
	)
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func serverList(client *client.Client, flags *flagSet, cmd *cobra.Command, args []string) {
	if len(args) > 0 {
		cmd.Usage()
		return
	}
	srvs, err := client.GetServers()
	if err != nil {
		log.Fatal(err)
	}

	for _, srv := range srvs {
		addr := net.JoinHostPort(srv.BindAddress, strconv.Itoa(int(srv.Port)))
		fmt.Print(addr)
		if len(srv.Mounts) == 0 {
			fmt.Println(" (no mounts)")
			continue
		}
		fmt.Println()
		for _, mount := range srv.Mounts {
			fmt.Printf("  * %s: %s -> %s\n", mount.ID, mount.Source, mount.Target)
		}
		fmt.Println()
	}
}

func serverAdd(client *client.Client, flags *flagSet, cmd *cobra.Command, args []string) {
	if len(args) > 1 {
		cmd.Usage()
		return
	}
	var (
		srv *admin.Server
		err error
	)
	if len(args) == 0 {
		srv, err = client.AddServerWithRandomPort(admin.CreateServerRequest{BindAddress: flags.ServerAddBind})
	} else {
		var port int
		port, err = strconv.Atoi(args[0])
		if err != nil {
			log.Fatalf("error parsing port: %v", err)
		}
		srv, err = client.AddServer(uint16(port), admin.CreateServerRequest{BindAddress: flags.ServerAddBind})
	}
	if err != nil {
		log.Fatal(err)
	}
	addr := net.JoinHostPort(srv.BindAddress, strconv.Itoa(int(srv.Port)))
	fmt.Printf("server available on %s\n", addr)
}

func serverRm(client *client.Client, flags *flagSet, cmd *cobra.Command, args []string) {
	if len(args) != 1 {
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

func serverRmAll(client *client.Client, flags *flagSet, cmd *cobra.Command, args []string) {
	if len(args) > 0 {
		cmd.Usage()
		return
	}
	if err := client.RemoveAllServers(); err != nil {
		log.Fatal(err)
	}
}

func fileServerList(client *client.Client, flags *flagSet, cmd *cobra.Command, args []string) {
	if len(args) > 0 {
		cmd.Usage()
		return
	}
	fsrvs, err := client.GetFileServers()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(strings.Join(fsrvs, ", "))
}

func mountList(client *client.Client, flags *flagSet, cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		cmd.Usage()
		return
	}
	port, err := strconv.Atoi(args[0])
	if err != nil {
		log.Fatalf("error parsing port: %v", err)
	}
	mounts, err := client.GetMounts(uint16(port))
	if err != nil {
		log.Fatal(err)
	}

	for _, mount := range mounts {
		fmt.Printf("%s: %s -> %s\n", mount.ID, mount.Source, mount.Target)
	}
}

func mountAdd(client *client.Client, flags *flagSet, cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		cmd.Usage()
		return
	}
	port, err := strconv.Atoi(args[0])
	if err != nil {
		log.Fatalf("error parsing port: %v", err)
	}
	source, err := filepath.Abs(args[1])
	if err != nil {
		log.Fatal(err)
	}

	target := "/" + filepath.Base(source)
	if flags.MountTarget != "" {
		target = flags.MountTarget
	}
	var fsParams fileserver.Params
	if err := json.Unmarshal([]byte(flags.FileServerParams), &fsParams); err != nil {
		log.Fatal(err)
	}

	mount, err := client.AddMount(uint16(port), admin.CreateServerMountRequest{
		target,
		source,
		flags.FileServerType,
		fsParams,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s: %s -> %s\n", mount.ID, mount.Source, mount.Target)
}

func mountRm(client *client.Client, flags *flagSet, cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		cmd.Usage()
		return
	}
	port, err := strconv.Atoi(args[0])
	if err != nil {
		log.Fatalf("error parsing port: %v", err)
	}
	mountID := args[1]
	if err := client.RemoveMount(uint16(port), mountID); err != nil {
		log.Fatal(err)
	}
}

func listenEvents(client *client.Client, flags *flagSet, cmd *cobra.Command, args []string) {
	events := args
	eventsCh := make(chan *admin.Event)
	go func() {
		if err := client.ListenEvents(eventsCh, events...); err != nil {
			log.Fatal(err)
		}
	}()
	for evt := range eventsCh {
		switch evt.Type {
		case admin.EventTypeServerAdd:
			se := evt.Resource.(*admin.ServerEvent)
			fmt.Printf("server added on http://%s:%d\n", se.Server.BindAddress, se.Server.Port)
		case admin.EventTypeServerRemove:
			se := evt.Resource.(*admin.ServerEvent)
			fmt.Printf("server removed on http://%s:%d\n", se.Server.BindAddress, se.Server.Port)
		case admin.EventTypeMountAdd:
			me := evt.Resource.(*admin.MountEvent)
			fmt.Printf("mount point %s added: %q -> http://%s:%d%s\n", me.Mount.ID, me.Mount.Source, me.Server.BindAddress, me.Server.Port, me.Mount.Target)
		case admin.EventTypeMountUpdate:
			me := evt.Resource.(*admin.MountEvent)
			fmt.Printf("mount point %s updated: %q -> http://%s:%d%s\n", me.Mount.ID, me.Mount.Source, me.Server.BindAddress, me.Server.Port, me.Mount.Target)
		case admin.EventTypeMountRemove:
			me := evt.Resource.(*admin.MountEvent)
			fmt.Printf("mount point %s removed: %q X http://%s:%d%s\n", me.Mount.ID, me.Mount.Source, me.Server.BindAddress, me.Server.Port, me.Mount.Target)
		case admin.EventTypeFileServe:
			fse := evt.Resource.(*admin.FileServeEvent)
			fmt.Printf("file served on http://%s:%d - %d - %s\n", fse.Server.BindAddress, fse.Server.Port, fse.Code, fse.Path)
		}
	}
}
