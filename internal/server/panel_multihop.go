package server

import (
	"net/http"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

type multiHopProfileReq struct {
	Reverse *model.MultiHopReverse `json:"reverse"`
	Name    string                 `json:"name"`
	Nodes   *[]int64               `json:"nodes"`
	Transit *int64                 `json:"transit"`
	Exit    *int64                 `json:"exit"`
}

func (rt *Router) listMultiHopProfiles(w http.ResponseWriter, _ *http.Request) {
	profiles, err := rt.mgr.MultiHopProfiles()
	if err != nil {
		writeManagerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profiles)
}

func (rt *Router) createMultiHopProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		multiHopProfileReq
		Entry *int64 `json:"entry"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Nodes != nil {
		profile, err := rt.mgr.CreateMultiHopRoute(r.Context(), req.Name, *req.Nodes, req.Reverse)
		if err != nil {
			writeManagerErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, profile)
		return
	}
	if req.Entry == nil || req.Transit == nil || req.Exit == nil {
		writeErrCode(w, http.StatusBadRequest, "err.multiHopDistinct", "выберите один или несколько разных серверов для мульти-хопа")
		return
	}
	profile, err := rt.mgr.CreateMultiHopProfile(r.Context(), *req.Entry, req.Name, model.MultiHop{Transit: *req.Transit, Exit: *req.Exit})
	if err != nil {
		writeManagerErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, profile)
}

func (rt *Router) updateMultiHopProfile(w http.ResponseWriter, r *http.Request, id int64) {
	var req struct {
		multiHopProfileReq
		Enabled bool `json:"enabled"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Nodes != nil {
		profile, err := rt.mgr.UpdateMultiHopRoute(r.Context(), id, req.Name, *req.Nodes, req.Enabled, req.Reverse)
		if err != nil {
			writeManagerErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, profile)
		return
	}
	if req.Transit == nil || req.Exit == nil {
		writeErrCode(w, http.StatusBadRequest, "err.multiHopDistinct", "выберите один или несколько разных серверов для мульти-хопа")
		return
	}
	profile, err := rt.mgr.UpdateMultiHopProfile(r.Context(), id, req.Name, model.MultiHop{Transit: *req.Transit, Exit: *req.Exit}, req.Enabled)
	if err != nil {
		writeManagerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (rt *Router) deleteMultiHopProfile(w http.ResponseWriter, _ *http.Request, id int64) {
	if err := rt.mgr.DeleteMultiHopProfile(id); err != nil {
		writeManagerErr(w, err)
		return
	}
	writeOK(w)
}
