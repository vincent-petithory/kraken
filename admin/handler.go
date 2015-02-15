package admin

//func (sph *ServerPoolHandler) writeLocation(w http.ResponseWriter, route Route) {
//	u, err := sph.routes.RouteURL(route)
//	if err != nil {
//		sph.logErr(err)
//		return
//	}
//	w.Header().Set("Location", u.String())
//}
//
//func (sph *ServerPoolHandler) serverOr404(w http.ResponseWriter, r *http.Request) *kraken.Server {
//	sport := mux.Vars(r)["port"]
//	port, err := strconv.Atoi(sport)
//	if err != nil {
//		http.Error(w, err.Error(), http.StatusBadRequest)
//		return nil
//	}
//	srv := sph.ServerPool.Get(uint16(port))
//	if srv == nil {
//		http.Error(w, fmt.Sprintf("server %d not found", port), http.StatusNotFound)
//		return nil
//	}
//	return srv
//}
//
//func (sph *ServerPoolHandler) getServers(w http.ResponseWriter, r *http.Request) {
//	spSrvs := sph.ServerPool.Servers()
//	srvs := make([]Server, 0, len(spSrvs))
//	for _, srv := range spSrvs {
//		srvs = append(srvs, *newServerDataFromServer(srv))
//	}
//	sph.serveJSON(w, r, srvs, http.StatusOK)
//}
//
//func (sph *ServerPoolHandler) getServer(w http.ResponseWriter, r *http.Request) {
//	srv := sph.serverOr404(w, r)
//	if srv == nil {
//		return
//	}
//	sph.serveJSON(w, r, newServerDataFromServer(srv), http.StatusOK)
//}
//
//
//func (sph *ServerPoolHandler) createServerWithRandomPort(w http.ResponseWriter, r *http.Request) {
//	var req CreateServerRequest
//	defer r.Body.Close()
//	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
//		http.Error(w, err.Error(), http.StatusBadRequest)
//		return
//	}
//
//	srv, ok := sph.addAndStartSrv(w, r, req.BindAddress, "0")
//	if !ok {
//		return
//	}
//
//	sph.writeLocation(w, ServersSelfRoute{srv.Port})
//	sph.serveJSON(w, r, newServerDataFromServer(srv), http.StatusCreated)
//}
//
//func (sph *ServerPoolHandler) createServer(w http.ResponseWriter, r *http.Request) {
//	var req CreateServerRequest
//	defer r.Body.Close()
//	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
//		http.Error(w, err.Error(), http.StatusBadRequest)
//		return
//	}
//	sport := mux.Vars(r)["port"]
//	port, err := strconv.Atoi(sport)
//	if err != nil {
//		http.Error(w, err.Error(), http.StatusBadRequest)
//		return
//	}
//
//	srv, ok := sph.addAndStartSrv(w, r, req.BindAddress, strconv.Itoa(port))
//	if !ok {
//		return
//	}
//
//	sph.serveJSON(w, r, newServerDataFromServer(srv), http.StatusOK)
//}
//
//func (sph *ServerPoolHandler) removeServers(w http.ResponseWriter, r *http.Request) {
//	var errs []error
//	for _, srv := range sph.ServerPool.Servers() {
//		if ok, err := sph.ServerPool.Remove(srv.Port); err != nil {
//			errs = append(errs, err)
//			sph.logErrSrv(srv, err)
//		} else if !ok {
//			err := fmt.Errorf("unable to shut down server on port %d", srv.Port)
//			sph.logErrSrv(srv, "unable to shut down server")
//			errs = append(errs, err)
//		} else {
//			sph.logfSrv(srv, "server shut down")
//		}
//		sph.events.Send(Event{EventTypeServerRemove, ServerEvent{*newServerDataFromServer(srv)}})
//	}
//	if len(errs) > 0 {
//		var bufMsg bytes.Buffer
//		for _, err := range errs {
//			fmt.Fprintln(&bufMsg, err.Error())
//		}
//		w.WriteHeader(http.StatusInternalServerError)
//		http.Error(w, bufMsg.String(), http.StatusInternalServerError)
//		return
//	}
//	w.WriteHeader(http.StatusOK)
//}
//
//func (sph *ServerPoolHandler) removeServer(w http.ResponseWriter, r *http.Request) {
//	srv := sph.serverOr404(w, r)
//	if srv == nil {
//		return
//	}
//	if ok, err := sph.ServerPool.Remove(srv.Port); err != nil {
//		http.Error(w, err.Error(), http.StatusInternalServerError)
//		return
//	} else if !ok {
//		sph.logErrSrv(srv, "unable to shut down server")
//		http.Error(w, fmt.Sprintf("unable to shut down server on port %d", srv.Port), http.StatusInternalServerError)
//		return
//	} else {
//		sph.logfSrv(srv, "server shut down")
//	}
//	sph.events.Send(Event{EventTypeServerRemove, ServerEvent{*newServerDataFromServer(srv)}})
//	w.WriteHeader(http.StatusOK)
//}
//
//func (sph *ServerPoolHandler) getServerMounts(w http.ResponseWriter, r *http.Request) {
//	srv := sph.serverOr404(w, r)
//	if srv == nil {
//		return
//	}
//	mountTargets := srv.MountMap.Targets()
//	mounts := make([]Mount, 0, len(mountTargets))
//	for _, mountTarget := range mountTargets {
//		mounts = append(mounts, Mount{
//			ID:     mountID(mountTarget),
//			Source: srv.MountMap.GetSource(mountTarget),
//			Target: mountTarget,
//		})
//	}
//	sph.serveJSON(w, r, mounts, http.StatusOK)
//}
//
//func (sph *ServerPoolHandler) removeServerMounts(w http.ResponseWriter, r *http.Request) {
//	srv := sph.serverOr404(w, r)
//	if srv == nil {
//		return
//	}
//	mountTargets := srv.MountMap.Targets()
//	for _, mountTarget := range mountTargets {
//		mount := Mount{
//			ID:     mountID(mountTarget),
//			Source: srv.MountMap.GetSource(mountTarget),
//			Target: mountTarget,
//		}
//		if ok := srv.MountMap.DeleteTarget(mountTarget); ok {
//			sph.logfSrv(srv, "removed mount point %s", mountID(mountTarget))
//			sph.events.Send(Event{EventTypeMountRemove, MountEvent{*newServerDataFromServer(srv), mount}})
//		}
//	}
//	w.WriteHeader(http.StatusOK)
//}
//
//func (sph *ServerPoolHandler) getServerMount(w http.ResponseWriter, r *http.Request) {
//	srv := sph.serverOr404(w, r)
//	if srv == nil {
//		return
//	}
//	reqMountID := mux.Vars(r)["mount"]
//	var mountTarget string
//	for _, mt := range srv.MountMap.Targets() {
//		if mountID(mt) == reqMountID {
//			mountTarget = mt
//			break
//		}
//	}
//	mountSource := srv.MountMap.GetSource(mountTarget)
//	if mountSource == "" {
//		http.Error(w, fmt.Sprintf("server %d has no mount target %q", srv.Port, mountTarget), http.StatusNotFound)
//		return
//	}
//
//	mount := Mount{
//		ID:     mountID(mountTarget),
//		Source: srv.MountMap.GetSource(mountTarget),
//		Target: mountTarget,
//	}
//	sph.serveJSON(w, r, mount, http.StatusOK)
//}
//
//func (sph *ServerPoolHandler) createServerMount(w http.ResponseWriter, r *http.Request) {
//	srv := sph.serverOr404(w, r)
//	if srv == nil {
//		return
//	}
//	defer r.Body.Close()
//	var req CreateServerMountRequest
//	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
//		http.Error(w, err.Error(), http.StatusBadRequest)
//		return
//	}
//
//	exists, err := srv.MountMap.Put(req.Target, req.Source, req.FsType, req.FsParams)
//	if err != nil {
//		http.Error(w, err.Error(), http.StatusBadRequest)
//		return
//	}
//
//	mount := Mount{
//		ID:     mountID(req.Target),
//		Source: srv.MountMap.GetSource(req.Target),
//		Target: req.Target,
//	}
//	if exists {
//		sph.logfSrv(srv, "updated mount point %s: mount %s on http://%s%s", mount.ID, mount.Source, srv.Addr, mount.Target)
//		sph.events.Send(Event{EventTypeMountUpdate, MountEvent{*newServerDataFromServer(srv), mount}})
//	} else {
//		sph.logfSrv(srv, "created mount point %s: mount %s on http://%s%s", mount.ID, mount.Source, srv.Addr, mount.Target)
//		sph.events.Send(Event{EventTypeMountAdd, MountEvent{*newServerDataFromServer(srv), mount}})
//	}
//
//	sph.writeLocation(w, ServersSelfMountsSelfRoute{Port: srv.Port, ID: mount.ID})
//	sph.serveJSON(w, r, mount, http.StatusCreated)
//}
//
//func (sph *ServerPoolHandler) removeServerMount(w http.ResponseWriter, r *http.Request) {
//	srv := sph.serverOr404(w, r)
//	if srv == nil {
//		return
//	}
//	reqMountID := mux.Vars(r)["mount"]
//	var mountTarget string
//	for _, mt := range srv.MountMap.Targets() {
//		if mountID(mt) == reqMountID {
//			mountTarget = mt
//			break
//		}
//	}
//	mount := Mount{
//		ID:     reqMountID,
//		Source: srv.MountMap.GetSource(mountTarget),
//		Target: mountTarget,
//	}
//	ok := srv.MountMap.DeleteTarget(mountTarget)
//	if !ok {
//		http.Error(w, fmt.Sprintf("server %d has no mount target %q", srv.Port, mountTarget), http.StatusNotFound)
//		return
//	}
//	sph.logfSrv(srv, "removed mount point %s", reqMountID)
//	sph.events.Send(Event{EventTypeMountRemove, MountEvent{*newServerDataFromServer(srv), mount}})
//
//	w.WriteHeader(http.StatusOK)
//}
//
//func (sph *ServerPoolHandler) getFileServers(w http.ResponseWriter, r *http.Request) {
//	sph.serveJSON(w, r, FileServerTypes(sph.ServerPool.Fsf.Types()), http.StatusOK)
//}
