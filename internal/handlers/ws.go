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
	var rawRooms []string
	if roomsParam := r.URL.Query().Get("rooms"); roomsParam != "" {
		rawRooms = strings.Split(roomsParam, ",")
	}

	t := middleware.GetTenantFromContext(r.Context())
	var sanitizedRooms []string

	if t != nil {
		activeTenantRoom := "tenant:" + t.ID.String()
		sanitizedRooms = append(sanitizedRooms, activeTenantRoom)

		for _, room := range rawRooms {
			room = strings.TrimSpace(room)
			if room == "" || room == activeTenantRoom {
				continue
			}
			// Security: Block joining foreign tenant rooms
			if strings.HasPrefix(room, "tenant:") {
				continue
			}
			sanitizedRooms = append(sanitizedRooms, room)
		}
	} else {
		for _, room := range rawRooms {
			room = strings.TrimSpace(room)
			if room != "" {
				sanitizedRooms = append(sanitizedRooms, room)
			}
		}
	}

	realtime.ServeWs(h.hub, w, r, sanitizedRooms)
}
