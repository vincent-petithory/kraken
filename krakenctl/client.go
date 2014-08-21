package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/vincent-petithory/kraken/admin"
)

type client struct {
	c      *http.Client
	url    *url.URL
	routes *admin.ServerPoolAdminRoutes
}

func (c *client) newRequest(method string, route admin.Route, v interface{}) (*http.Request, error) {
	u := *c.url
	u.Path = route.URL(c.routes).Path
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

func (c *client) doRequest(method string, route admin.Route, v interface{}) (*http.Response, error) {
	r, err := c.newRequest(method, route, v)
	if err != nil {
		return nil, err
	}
	resp, err := c.c.Do(r)
	if err != nil {
		return nil, err
	}
	return resp, err
}

func (c *client) checkCode(resp *http.Response, code int) error {
	// TODO unmarshal api error type when jsonify4xx-5xx middleware is done.
	if resp.StatusCode != code {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("error %d: %s\n", resp.StatusCode, b)
	}
	return nil
}

func (c *client) AddServerWithRandomPort(bindAddr string) error {
	data := admin.CreateServerRequest{BindAddress: bindAddr}
	resp, err := c.doRequest("POST", admin.ServersRoute{}, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := c.checkCode(resp, http.StatusCreated); err != nil {
		return err
	}
	var srv admin.Server
	if err := json.NewDecoder(resp.Body).Decode(&srv); err != nil {
		return err
	}
	addr := net.JoinHostPort(srv.BindAddress, strconv.Itoa(int(srv.Port)))
	fmt.Printf("server available on %s\n", addr)
	return nil
}

func (c *client) AddServer(bindAddr string, port uint16) error {
	data := admin.CreateServerRequest{BindAddress: bindAddr}
	resp, err := c.doRequest("PUT", admin.ServersSelfRoute{port}, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := c.checkCode(resp, http.StatusOK); err != nil {
		return err
	}
	var srv admin.Server
	if err := json.NewDecoder(resp.Body).Decode(&srv); err != nil {
		return err
	}
	addr := net.JoinHostPort(srv.BindAddress, strconv.Itoa(int(srv.Port)))
	fmt.Printf("server available on %s\n", addr)
	return nil
}

func (c *client) RemoveServer(port uint16) error {
	resp, err := c.doRequest("DELETE", admin.ServersSelfRoute{port}, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := c.checkCode(resp, http.StatusOK); err != nil {
		return err
	}
	return nil
}

func (c *client) RemoveAllServers() error {
	resp, err := c.doRequest("DELETE", admin.ServersRoute{}, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := c.checkCode(resp, http.StatusOK); err != nil {
		return err
	}
	return nil
}
