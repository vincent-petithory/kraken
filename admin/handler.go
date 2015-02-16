package admin

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/vincent-petithory/kraken/fileserver"
)

func (sph *ServerPoolHandler) writeLocation(w http.ResponseWriter, routeLocation RouteLocation) {
	w.Header().Set("Location", routeLocation.Location(sph.router).String())
}

func (sph *ServerPoolHandler) getFileservers(w http.ResponseWriter, r *http.Request) (int, ListAllFileServerTypeOut, error) {
	return http.StatusOK, ListAllFileServerTypeOut(sph.ServerPool.Fsf.Types()), nil
}

func (sph *ServerPoolHandler) getServers(w http.ResponseWriter, r *http.Request) (int, ListAllServerOut, error) {
	spSrvs := sph.ServerPool.Servers()
	srvs := make([]Server, len(spSrvs))
	for i, srv := range spSrvs {
		srvs[i] = *newServerDataFromServer(srv)
	}
	return http.StatusOK, srvs, nil
}

func (sph *ServerPoolHandler) postServers(w http.ResponseWriter, r *http.Request, vreq *CreateRandomServerIn) (int, *Server, error) {
	srv, err := sph.addAndStartSrv(vreq.BindAddress, "0")
	if err != nil {
		return http.StatusInternalServerError, nil, err
	}

	sph.writeLocation(w, RouteServersOne{
		ServerPort: strconv.Itoa(int(srv.Port)),
	})
	return http.StatusCreated, newServerDataFromServer(srv), nil
}

func (sph *ServerPoolHandler) deleteServers(w http.ResponseWriter, r *http.Request) (int, DeleteAllServerOut, error) {
	var (
		errs []error
		srvs []Server
	)
	for _, srv := range sph.ServerPool.Servers() {
		srvData := newServerDataFromServer(srv)
		if ok, err := sph.ServerPool.Remove(srv.Port); err != nil {
			errs = append(errs, err)
			sph.logErrSrv(srv, err)
		} else if !ok {
			err := fmt.Errorf("unable to shut down server on port %d", srv.Port)
			sph.logErrSrv(srv, "unable to shut down server")
			errs = append(errs, err)
		} else {
			sph.logfSrv(srv, "server shut down")
			srvs = append(srvs, *srvData)
		}
		sph.events.Send(Event{EventTypeServerRemove, ServerEvent{*srvData}})
	}
	if len(errs) > 0 {
		var bufMsg bytes.Buffer
		for _, err := range errs {
			fmt.Fprintln(&bufMsg, err.Error())
		}
		return http.StatusInternalServerError, nil, errors.New(bufMsg.String())
	}
	return http.StatusOK, srvs, nil
}

func (sph *ServerPoolHandler) getServersOne(w http.ResponseWriter, r *http.Request, serverPort string) (int, *Server, error) {
	port, err := strconv.Atoi(serverPort)
	if err != nil {
		return http.StatusBadRequest, nil, err
	}
	srv := sph.ServerPool.Get(uint16(port))
	if srv == nil {
		return http.StatusNotFound, nil, fmt.Errorf("server %q not found", serverPort)
	}
	return http.StatusOK, newServerDataFromServer(srv), nil
}

func (sph *ServerPoolHandler) putServersOne(w http.ResponseWriter, r *http.Request, serverPort string, vreq *CreateServerIn) (int, *Server, error) {
	port, err := strconv.Atoi(serverPort)
	if err != nil {
		return http.StatusBadRequest, nil, err
	}
	srv, err := sph.addAndStartSrv(vreq.BindAddress, strconv.Itoa(port))
	if err != nil {
		return http.StatusInternalServerError, nil, err
	}
	return http.StatusOK, newServerDataFromServer(srv), nil
}

func (sph *ServerPoolHandler) deleteServersOne(w http.ResponseWriter, r *http.Request, serverPort string) (int, *Server, error) {
	port, err := strconv.Atoi(serverPort)
	if err != nil {
		return http.StatusBadRequest, nil, err
	}
	srv := sph.ServerPool.Get(uint16(port))
	if srv == nil {
		return http.StatusNotFound, nil, fmt.Errorf("server %q not found", serverPort)
	}
	srvData := newServerDataFromServer(srv)
	if ok, err := sph.ServerPool.Remove(srv.Port); err != nil {
		return http.StatusInternalServerError, nil, err
	} else if !ok {
		sph.logErrSrv(srv, "unable to shut down server")
		return http.StatusInternalServerError, nil, fmt.Errorf("unable to shut down server on port %d", srv.Port)
	} else {
		sph.logfSrv(srv, "server shut down")
	}
	sph.events.Send(Event{EventTypeServerRemove, ServerEvent{*srvData}})
	return http.StatusNotImplemented, srvData, nil
}

func (sph *ServerPoolHandler) getServersOneMounts(w http.ResponseWriter, r *http.Request, serverPort string) (int, ListAllMountOut, error) {
	port, err := strconv.Atoi(serverPort)
	if err != nil {
		return http.StatusBadRequest, nil, err
	}
	srv := sph.ServerPool.Get(uint16(port))
	if srv == nil {
		return http.StatusNotFound, nil, fmt.Errorf("server %q not found", serverPort)
	}
	mountTargets := srv.MountMap.Targets()
	mounts := make([]Mount, 0, len(mountTargets))
	for _, mountTarget := range mountTargets {
		mounts = append(mounts, Mount{
			Id:     mountID(mountTarget),
			Source: srv.MountMap.GetSource(mountTarget),
			Target: mountTarget,
		})
	}
	return http.StatusOK, ListAllMountOut(mounts), nil
}

func (sph *ServerPoolHandler) postServersOneMounts(w http.ResponseWriter, r *http.Request, serverPort string, vreq *CreateMountIn) (int, *Mount, error) {
	port, err := strconv.Atoi(serverPort)
	if err != nil {
		return http.StatusBadRequest, nil, err
	}
	srv := sph.ServerPool.Get(uint16(port))
	if srv == nil {
		return http.StatusNotFound, nil, fmt.Errorf("server %q not found", serverPort)
	}
	exists, err := srv.MountMap.Put(vreq.Target, vreq.Source, vreq.FsType, fileserver.Params(vreq.FsParams))
	if err != nil {
		return http.StatusBadRequest, nil, err
	}

	mount := Mount{
		Id:     mountID(vreq.Target),
		Source: srv.MountMap.GetSource(vreq.Target),
		Target: vreq.Target,
	}
	if exists {
		sph.logfSrv(srv, "updated mount point %s: mount %s on http://%s%s", mount.Id, mount.Source, srv.Addr, mount.Target)
		sph.events.Send(Event{EventTypeMountUpdate, MountEvent{*newServerDataFromServer(srv), mount}})
	} else {
		sph.logfSrv(srv, "created mount point %s: mount %s on http://%s%s", mount.Id, mount.Source, srv.Addr, mount.Target)
		sph.events.Send(Event{EventTypeMountAdd, MountEvent{*newServerDataFromServer(srv), mount}})
	}

	sph.writeLocation(w, RouteServersOneMountsOne{ServerPort: strconv.Itoa(int(srv.Port)), MountId: mount.Id})
	return http.StatusCreated, &mount, nil
}

func (sph *ServerPoolHandler) deleteServersOneMounts(w http.ResponseWriter, r *http.Request, serverPort string) (int, DeleteAllMountOut, error) {
	port, err := strconv.Atoi(serverPort)
	if err != nil {
		return http.StatusBadRequest, nil, err
	}
	srv := sph.ServerPool.Get(uint16(port))
	if srv == nil {
		return http.StatusNotFound, nil, fmt.Errorf("server %q not found", serverPort)
	}
	mountTargets := srv.MountMap.Targets()
	var mounts []Mount
	for _, mountTarget := range mountTargets {
		mount := Mount{
			Id:     mountID(mountTarget),
			Source: srv.MountMap.GetSource(mountTarget),
			Target: mountTarget,
		}
		if ok := srv.MountMap.DeleteTarget(mountTarget); ok {
			sph.logfSrv(srv, "removed mount point %s", mountID(mountTarget))
			sph.events.Send(Event{EventTypeMountRemove, MountEvent{*newServerDataFromServer(srv), mount}})
			mounts = append(mounts, mount)
		}
	}
	return http.StatusOK, mounts, nil
}

func (sph *ServerPoolHandler) getServersOneMountsOne(w http.ResponseWriter, r *http.Request, serverPort string, mountId string) (int, *Mount, error) {
	port, err := strconv.Atoi(serverPort)
	if err != nil {
		return http.StatusBadRequest, nil, err
	}
	srv := sph.ServerPool.Get(uint16(port))
	if srv == nil {
		return http.StatusNotFound, nil, fmt.Errorf("server %q not found", serverPort)
	}
	var mountTarget string
	for _, mt := range srv.MountMap.Targets() {
		if mountID(mt) == mountId {
			mountTarget = mt
			break
		}
	}
	mountSource := srv.MountMap.GetSource(mountTarget)
	if mountSource == "" {
		return http.StatusNotFound, nil, fmt.Errorf("server %d has no mount target %q", srv.Port, mountTarget)
	}

	mount := Mount{
		Id:     mountID(mountTarget),
		Source: srv.MountMap.GetSource(mountTarget),
		Target: mountTarget,
	}
	return http.StatusOK, &mount, nil
}

func (sph *ServerPoolHandler) deleteServersOneMountsOne(w http.ResponseWriter, r *http.Request, serverPort string, mountId string) (int, *Mount, error) {
	port, err := strconv.Atoi(serverPort)
	if err != nil {
		return http.StatusBadRequest, nil, err
	}
	srv := sph.ServerPool.Get(uint16(port))
	if srv == nil {
		return http.StatusNotFound, nil, fmt.Errorf("server %q not found", serverPort)
	}
	var mountTarget string
	for _, mt := range srv.MountMap.Targets() {
		if mountID(mt) == mountId {
			mountTarget = mt
			break
		}
	}
	mount := Mount{
		Id:     mountId,
		Source: srv.MountMap.GetSource(mountTarget),
		Target: mountTarget,
	}
	ok := srv.MountMap.DeleteTarget(mountTarget)
	if !ok {
		return http.StatusNotFound, nil, fmt.Errorf("server %d has no mount target %q", srv.Port, mountTarget)
	}
	sph.logfSrv(srv, "removed mount point %s", mountId)
	sph.events.Send(Event{EventTypeMountRemove, MountEvent{*newServerDataFromServer(srv), mount}})

	return http.StatusOK, &mount, nil
}
