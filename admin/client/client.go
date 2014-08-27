package client

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

// Client defines methods to access the Kraken RESTful API.
type Client struct {
	C      http.Client // HTTP Client
	url    *url.URL
	routes *admin.ServerPoolRoutes
}

// New returns a Client which will hit the API at apiURL.
func New(apiURL *url.URL) *Client {
	return &Client{
		C:      http.Client{},
		url:    apiURL,
		routes: admin.NewServerPoolRoutes(),
	}
}

func (c *Client) newRequest(method string, route admin.Route, v interface{}) (*http.Request, error) {
	u := *c.url
	ru, err := route.URL(c.routes)
	if err != nil {
		return nil, err
	}
	u.Path = ru.Path
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

func (c *Client) doRequest(method string, route admin.Route, v interface{}) (*http.Response, error) {
	r, err := c.newRequest(method, route, v)
	if err != nil {
		return nil, err
	}
	resp, err := c.C.Do(r)
	if err != nil {
		return nil, err
	}
	return resp, err
}

func (c *Client) checkCode(resp *http.Response, code int) error {
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

func (c *Client) doRequestAndDecodeResponse(method string, route admin.Route, reqData interface{}, code int, respData interface{}) error {
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

func (c *Client) GetServers() ([]admin.Server, error) {
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

func (c *Client) GetServer(port uint16) (*admin.Server, error) {
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

func (c *Client) AddServerWithRandomPort(data admin.CreateServerRequest) (*admin.Server, error) {
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

func (c *Client) AddServer(port uint16, data admin.CreateServerRequest) (*admin.Server, error) {
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

func (c *Client) RemoveServer(port uint16) error {
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

func (c *Client) RemoveAllServers() error {
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

func (c *Client) GetFileServers() (admin.FileServerTypes, error) {
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

func (c *Client) GetMounts(port uint16) ([]admin.Mount, error) {
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

func (c *Client) GetMount(port uint16, mountID string) (*admin.Mount, error) {
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

func (c *Client) AddMount(port uint16, data admin.CreateServerMountRequest) (*admin.Mount, error) {
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

func (c *Client) RemoveAllMounts(port uint16) error {
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

func (c *Client) RemoveMount(port uint16, mountID string) error {
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
