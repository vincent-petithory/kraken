kraken is a on-demand http server: you can create new http servers at runtime through a RESTful API or one of the available clients.
Those servers are meant to serve static files.
It needs almost no configuration and can (will) save state across restarts.

Typical uses are sharing files quickly, on LAN or from a remote box.
If you happen to often use `python -m http.server`, then this project should be of interest.

## Install

Kraken is written in Go. The first step is to install Go on your system, if it isn't done yet.

Go is packaged for most linux distributions, but you can install it from [a binary distribution or from source](http://golang.org/doc/install).
To install the *kraken* programs, do `go get github.com/vincent-petithory/kraken/...`
You can update an installation of *kraken* with `go get -u github.com/vincent-petithory/kraken/...`

Upon install, `krakend` and `krakenctl` are available.

## Quickstart

~~~ shell
$ # Start the server
$ krakend
2014/08/22 16:50:17 [admin] Listening on 127.0.0.1:4214
2014/08/22 16:50:17 [admin] Available on http://127.0.0.1:4214
~~~

Here's a sample session, using the `krakenctl` client.
It creates a http server and makes it serve `$HOME/Pictures` on `http://localhost:4567/pics`.

~~~ shell
$ # Create a http server listening on port 4567, and bind it to localhost.
$ krakenctl add --bind=localhost 4567
server available on 127.0.0.1:4567
$ # Make it serve $HOME/Pictures mounted on /pics
$ krakenctl mount 4567 --target=/pics $HOME/Pictures
8f71ae0: /home/meow/Pictures -> /pics
$ # Print a status
$ krakenctl ls
127.0.0.1:4567
  * 8f71ae0: /home/vincent/Pictures -> /pics
$ # View contents in a browser
$ xdg-open http://localhost:4567/pics
~~~
