package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/vincent-petithory/kraken/admin"
)

type client struct {
	c   *http.Client
	url *url.URL
}

func (c *client) newRequest(method string, path string, v interface{}) (*http.Request, error) {
	u := *c.url
	u.Path = path
	var body io.Reader
	if v != nil {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	r, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if v != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	return r, nil
}

func (c *client) AddServerWithRandomPort(bindAddr string) error {
	return nil
}

func (c *client) AddServer(bindAddr string, port uint16) error {
	data := admin.CreateServerRequest{BindAddress: bindAddr}
	r, err := c.newRequest("PUT", fmt.Sprintf("/api/servers/%d", port), data)
	resp, err := c.c.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error %d: %s\n", resp.StatusCode, resp.Status)
	}
	var srv admin.Server
	if err := json.NewDecoder(resp.Body).Decode(&srv); err != nil {
		return err
	}
	addr := net.JoinHostPort(srv.BindAddress, strconv.Itoa(int(srv.Port)))
	fmt.Printf("server available on %s\n", addr)
	return nil
}
