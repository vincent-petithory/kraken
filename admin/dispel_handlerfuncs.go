// generated by dispel v1; DO NOT EDIT

package admin

import (
	"net/http"
)

func (sph *ServerPoolHandler) getFileservers(w http.ResponseWriter, r *http.Request) (int, ListAllFileServerTypeOut, error) {
	return http.StatusNotImplemented, nil, nil
}

func (sph *ServerPoolHandler) getServers(w http.ResponseWriter, r *http.Request) (int, ListAllServerOut, error) {
	return http.StatusNotImplemented, nil, nil
}

func (sph *ServerPoolHandler) postServers(w http.ResponseWriter, r *http.Request, vreq *CreateRandomServerIn) (int, *Server, error) {
	return http.StatusNotImplemented, nil, nil
}

func (sph *ServerPoolHandler) deleteServers(w http.ResponseWriter, r *http.Request) (int, DeleteAllServerOut, error) {
	return http.StatusNotImplemented, nil, nil
}

func (sph *ServerPoolHandler) getServersOne(w http.ResponseWriter, r *http.Request, serverPort string) (int, *Server, error) {
	return http.StatusNotImplemented, nil, nil
}

func (sph *ServerPoolHandler) putServersOne(w http.ResponseWriter, r *http.Request, serverPort string, vreq *CreateServerIn) (int, *Server, error) {
	return http.StatusNotImplemented, nil, nil
}

func (sph *ServerPoolHandler) deleteServersOne(w http.ResponseWriter, r *http.Request, serverPort string) (int, *Server, error) {
	return http.StatusNotImplemented, nil, nil
}

func (sph *ServerPoolHandler) getServersOneMounts(w http.ResponseWriter, r *http.Request, serverPort string) (int, ListAllMountOut, error) {
	return http.StatusNotImplemented, nil, nil
}

func (sph *ServerPoolHandler) postServersOneMounts(w http.ResponseWriter, r *http.Request, serverPort string, vreq *CreateMountIn) (int, *Mount, error) {
	return http.StatusNotImplemented, nil, nil
}

func (sph *ServerPoolHandler) deleteServersOneMounts(w http.ResponseWriter, r *http.Request, serverPort string) (int, DeleteAllMountOut, error) {
	return http.StatusNotImplemented, nil, nil
}

func (sph *ServerPoolHandler) getServersOneMountsOne(w http.ResponseWriter, r *http.Request, serverPort string, mountId string) (int, *Mount, error) {
	return http.StatusNotImplemented, nil, nil
}

func (sph *ServerPoolHandler) deleteServersOneMountsOne(w http.ResponseWriter, r *http.Request, serverPort string, mountId string) (int, *Mount, error) {
	return http.StatusNotImplemented, nil, nil
}
