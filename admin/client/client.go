package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vincent-petithory/kraken/admin"
)

// Client defines methods to access the Kraken RESTful API.
type Client struct {
	C             http.Client // HTTP Client
	WSC           websocket.Dialer
	routeReverser admin.RouteReverser
}

// New returns a Client which will hit the API at apiURL.
func New(apiURL *url.URL) *Client {
	return &Client{
		C: http.Client{},
		WSC: websocket.Dialer{
			ReadBufferSize:  1 << 10,
			WriteBufferSize: 1 << 8,
		},
		routeReverser: admin.NewServerPoolRoutes(apiURL),
	}
}

func (c *Client) newRequest(method string, route admin.RouteLocation, v interface{}) (*http.Request, error) {
	u := route.Location(c.routeReverser)
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

func (c *Client) doRequest(method string, route admin.RouteLocation, v interface{}) (*http.Response, error) {
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
	if resp.StatusCode != code {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("error %d: %s\n", resp.StatusCode, b)
	}
	return nil
}

func (c *Client) doRequestAndDecodeResponse(method string, route admin.RouteLocation, reqData interface{}, code int, respData interface{}) error {
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

//go:generate go run genfuncs.go
//dispel -f funcs.go.tmpl -o funcs.gen.go ../schema.json

//func (c *Client) GetServers() ([]admin.Server, error) {
//	var srvs []admin.Server
//	if err := c.doRequestAndDecodeResponse(
//		"GET",
//		admin.ServersRoute{},
//		nil,
//		http.StatusOK,
//		&srvs,
//	); err != nil {
//		return nil, err
//	}
//	return srvs, nil
//}
//
//func (c *Client) GetServer(port uint16) (*admin.Server, error) {
//	var srv admin.Server
//	if err := c.doRequestAndDecodeResponse(
//		"GET",
//		admin.ServersSelfRoute{port},
//		nil,
//		http.StatusOK,
//		&srv,
//	); err != nil {
//		return nil, err
//	}
//	return &srv, nil
//}
//
//func (c *Client) AddServerWithRandomPort(data admin.CreateServerRequest) (*admin.Server, error) {
//	var srv admin.Server
//	if err := c.doRequestAndDecodeResponse(
//		"POST",
//		admin.ServersRoute{},
//		data,
//		http.StatusCreated,
//		&srv,
//	); err != nil {
//		return nil, err
//	}
//	return &srv, nil
//}
//
//func (c *Client) AddServer(port uint16, data admin.CreateServerRequest) (*admin.Server, error) {
//	var srv admin.Server
//	if err := c.doRequestAndDecodeResponse(
//		"PUT",
//		admin.ServersSelfRoute{port},
//		data,
//		http.StatusOK,
//		&srv,
//	); err != nil {
//		return nil, err
//	}
//	return &srv, nil
//}
//
//func (c *Client) RemoveServer(port uint16) error {
//	if err := c.doRequestAndDecodeResponse(
//		"DELETE",
//		admin.ServersSelfRoute{port},
//		nil,
//		http.StatusOK,
//		nil,
//	); err != nil {
//		return err
//	}
//	return nil
//}
//
//func (c *Client) RemoveAllServers() error {
//	if err := c.doRequestAndDecodeResponse(
//		"DELETE",
//		admin.ServersRoute{},
//		nil,
//		http.StatusOK,
//		nil,
//	); err != nil {
//		return err
//	}
//	return nil
//}
//
//func (c *Client) GetFileServers() (admin.FileServerTypes, error) {
//	var fsrvs admin.FileServerTypes
//	if err := c.doRequestAndDecodeResponse(
//		"GET",
//		admin.FileServersRoute{},
//		nil,
//		http.StatusOK,
//		&fsrvs,
//	); err != nil {
//		return nil, err
//	}
//	return fsrvs, nil
//}
//
//func (c *Client) GetMounts(port uint16) ([]admin.Mount, error) {
//	var mounts []admin.Mount
//	if err := c.doRequestAndDecodeResponse(
//		"GET",
//		admin.ServersSelfMountsRoute{port},
//		nil,
//		http.StatusOK,
//		&mounts,
//	); err != nil {
//		return nil, err
//	}
//	return mounts, nil
//}
//
//func (c *Client) GetMount(port uint16, mountID string) (*admin.Mount, error) {
//	var mount admin.Mount
//	if err := c.doRequestAndDecodeResponse(
//		"GET",
//		admin.ServersSelfMountsSelfRoute{port, mountID},
//		nil,
//		http.StatusOK,
//		&mount,
//	); err != nil {
//		return nil, err
//	}
//	return &mount, nil
//}
//
//func (c *Client) AddMount(port uint16, data admin.CreateServerMountRequest) (*admin.Mount, error) {
//	var mount admin.Mount
//	if err := c.doRequestAndDecodeResponse(
//		"POST",
//		admin.ServersSelfMountsRoute{port},
//		data,
//		http.StatusCreated,
//		&mount,
//	); err != nil {
//		return nil, err
//	}
//	return &mount, nil
//}
//
//func (c *Client) RemoveAllMounts(port uint16) error {
//	if err := c.doRequestAndDecodeResponse(
//		"DELETE",
//		admin.ServersSelfMountsRoute{port},
//		nil,
//		http.StatusOK,
//		nil,
//	); err != nil {
//		return err
//	}
//	return nil
//}
//
//func (c *Client) RemoveMount(port uint16, mountID string) error {
//	if err := c.doRequestAndDecodeResponse(
//		"DELETE",
//		admin.ServersSelfMountsSelfRoute{port, mountID},
//		nil,
//		http.StatusOK,
//		nil,
//	); err != nil {
//		return err
//	}
//	return nil
//}

func (c *Client) ListenEvents(recvEvents chan *admin.Event, events ...string) error {
	u := admin.RouteEvents{}.Location(c.routeReverser)
	u.Scheme = "ws"
	if len(events) > 0 {
		var eventCodes []string
		for _, evt := range events {
			switch evt {
			case "server":
				eventCodes = append(eventCodes, []string{
					strconv.Itoa(int(admin.EventTypeServerAdd)),
					strconv.Itoa(int(admin.EventTypeServerRemove)),
				}...)
			case "mount":
				eventCodes = append(eventCodes, []string{
					strconv.Itoa(int(admin.EventTypeMountAdd)),
					strconv.Itoa(int(admin.EventTypeMountRemove)),
					strconv.Itoa(int(admin.EventTypeMountUpdate)),
				}...)
			case "fileserve":
				eventCodes = append(eventCodes, strconv.Itoa(int(admin.EventTypeFileServe)))
			default:
				return fmt.Errorf("unknown event %q", evt)
			}
		}
		v := url.Values{}
		v.Set(admin.EventsQueryKey, strings.Join(eventCodes, ","))
		u.RawQuery = v.Encode()
	}

	conn, _, err := c.WSC.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetPingHandler(func(string) error {
		conn.WriteControl(websocket.PongMessage, []byte{}, time.Now().Add(time.Second*10))
		return nil
	})
	for {
		mt, p, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		switch mt {
		case websocket.CloseMessage:
			return nil
		case websocket.TextMessage:
			var evt admin.Event
			if err := json.Unmarshal(p, &evt); err != nil {
				return err
			}
			recvEvents <- &evt
		}
	}
}
