package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/vincent-petithory/kraken/admin"
)

type client struct {
	c      *http.Client
	url    *url.URL
	routes *admin.ServerPoolRoutes
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

func (c *client) doRequestAndDecodeResponse(method string, route admin.Route, reqData interface{}, code int, respData interface{}) error {
	resp, err := c.doRequest(method, route, reqData)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := c.checkCode(resp, code); err != nil {
		return err
	}
	if respData == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return err
	}
	return nil
}

func (c *client) GetServers() ([]admin.Server, error) {
	var srvs []admin.Server
	if err := c.doRequestAndDecodeResponse(
		"GET",
		admin.ServersRoute{},
		nil,
		http.StatusOK,
		&srvs,
	); err != nil {
		return nil, err
	}
	return srvs, nil
}

func (c *client) GetServer(port uint16) (*admin.Server, error) {
	var srv admin.Server
	if err := c.doRequestAndDecodeResponse(
		"GET",
		admin.ServersSelfRoute{port},
		nil,
		http.StatusOK,
		&srv,
	); err != nil {
		return nil, err
	}
	return &srv, nil
}

func (c *client) AddServerWithRandomPort(data admin.CreateServerRequest) (*admin.Server, error) {
	var srv admin.Server
	if err := c.doRequestAndDecodeResponse(
		"POST",
		admin.ServersRoute{},
		data,
		http.StatusCreated,
		&srv,
	); err != nil {
		return nil, err
	}
	return &srv, nil
}

func (c *client) AddServer(port uint16, data admin.CreateServerRequest) (*admin.Server, error) {
	var srv admin.Server
	if err := c.doRequestAndDecodeResponse(
		"PUT",
		admin.ServersSelfRoute{port},
		data,
		http.StatusOK,
		&srv,
	); err != nil {
		return nil, err
	}
	return &srv, nil
}

func (c *client) RemoveServer(port uint16) error {
	if err := c.doRequestAndDecodeResponse(
		"DELETE",
		admin.ServersSelfRoute{port},
		nil,
		http.StatusOK,
		nil,
	); err != nil {
		return err
	}
	return nil
}

func (c *client) RemoveAllServers() error {
	if err := c.doRequestAndDecodeResponse(
		"DELETE",
		admin.ServersRoute{},
		nil,
		http.StatusOK,
		nil,
	); err != nil {
		return err
	}
	return nil
}

func (c *client) GetFileServers() (admin.FileServerTypes, error) {
	var fsrvs admin.FileServerTypes
	if err := c.doRequestAndDecodeResponse(
		"GET",
		admin.FileServersRoute{},
		nil,
		http.StatusOK,
		&fsrvs,
	); err != nil {
		return nil, err
	}
	return fsrvs, nil
}

func (c *client) GetMounts(port uint16) ([]admin.Mount, error) {
	var mounts []admin.Mount
	if err := c.doRequestAndDecodeResponse(
		"GET",
		admin.ServersSelfMountsRoute{port},
		nil,
		http.StatusOK,
		&mounts,
	); err != nil {
		return nil, err
	}
	return mounts, nil
}

func (c *client) GetMount(port uint16, mountID string) (*admin.Mount, error) {
	var mount admin.Mount
	if err := c.doRequestAndDecodeResponse(
		"GET",
		admin.ServersSelfMountsSelfRoute{port, mountID},
		nil,
		http.StatusOK,
		&mount,
	); err != nil {
		return nil, err
	}
	return &mount, nil
}

func (c *client) AddMount(port uint16, data admin.CreateServerMountRequest) (*admin.Mount, error) {
	var mount admin.Mount
	if err := c.doRequestAndDecodeResponse(
		"POST",
		admin.ServersSelfMountsRoute{port},
		data,
		http.StatusCreated,
		&mount,
	); err != nil {
		return nil, err
	}
	return &mount, nil
}

func (c *client) RemoveAllMounts(port uint16) error {
	if err := c.doRequestAndDecodeResponse(
		"DELETE",
		admin.ServersSelfMountsRoute{port},
		nil,
		http.StatusOK,
		nil,
	); err != nil {
		return err
	}
	return nil
}

func (c *client) RemoveMount(port uint16, mountID string) error {
	if err := c.doRequestAndDecodeResponse(
		"DELETE",
		admin.ServersSelfMountsSelfRoute{port, mountID},
		nil,
		http.StatusOK,
		nil,
	); err != nil {
		return err
	}
	return nil

}
