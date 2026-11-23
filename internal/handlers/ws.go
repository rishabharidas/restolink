package handlers

import (
	"net/http"
	"strings"

	"github.com/rishabharidas/restolink/internal/middleware"
	"github.com/rishabharidas/restolink/internal/realtime"
)

type WSHandler struct {
	hub *realtime.Hub
}

func NewWSHandler(hub *realtime.Hub) *WSHandler {
	return &WSHandler{hub: hub}
}

func (h *WSHandler) HandleConnection(w http.ResponseWriter, r *http.Request) {
	// Parse room query parameters, e.g. /ws?rooms=tenant:123,order:456
	var rooms []string

	if roomsParam := r.URL.Query().Get("rooms"); roomsParam != "" {
		rooms = strings.Split(roomsParam, ",")
	}

	// Also automatically join the current tenant room if resolved
	if t := middleware.GetTenantFromContext(r.Context()); t != nil {
		rooms = append(rooms, "tenant:"+t.ID.String())
	}

	realtime.ServeWs(h.hub, w, r, rooms)
}
