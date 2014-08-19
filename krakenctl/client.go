package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/vincent-petithory/kraken"
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
	data := kraken.CreateServerRequest{BindAddress: bindAddr}
	r, err := c.newRequest("PUT", fmt.Sprintf("/api/servers/%d", port), data)
	resp, err := c.c.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error %d: %s\n", resp.StatusCode, resp.Status)
	}
	var serverData kraken.ServerData
	if err := json.NewDecoder(resp.Body).Decode(&serverData); err != nil {
		return err
	}
	fmt.Printf("server available on %s:%d\n", serverData.BindAddress, serverData.Port)
	return nil
}
