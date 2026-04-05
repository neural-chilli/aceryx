package drivers

import (
	"net/http"
	"strings"
)

type AdminHandlers struct {
	Registry *DriverRegistry
	Pools    *PoolManager
}

func NewAdminHandlers(registry *DriverRegistry, pools *PoolManager) *AdminHandlers {
	if pools == nil {
		pools = NewPoolManager()
	}
	return &AdminHandlers{Registry: registry, Pools: pools}
}

func (h *AdminHandlers) ListDrivers(w http.ResponseWriter, _ *http.Request) {
	if h.Registry == nil {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"db":       describeDB(h.Registry.AllDB()),
		"queue":    describeQueue(h.Registry.AllQueues()),
		"file":     describeFile(h.Registry.AllFiles()),
		"protocol": describeProtocol(h.Registry.AllProtocols()),
		"smtp":     describeSMTP(h.Registry.AllSMTP()),
		"imap":     describeIMAP(h.Registry.AllIMAP()),
	})
}

func (h *AdminHandlers) ListDriversByCategory(w http.ResponseWriter, r *http.Request) {
	if h.Registry == nil {
		writeError(w, http.StatusNotFound, "driver_registry_not_configured")
		return
	}
	category := strings.TrimSpace(r.PathValue("category"))
	switch category {
	case "db":
		writeJSON(w, http.StatusOK, describeDB(h.Registry.AllDB()))
	case "queue":
		writeJSON(w, http.StatusOK, describeQueue(h.Registry.AllQueues()))
	case "file":
		writeJSON(w, http.StatusOK, describeFile(h.Registry.AllFiles()))
	case "protocol":
		writeJSON(w, http.StatusOK, describeProtocol(h.Registry.AllProtocols()))
	case "smtp":
		writeJSON(w, http.StatusOK, describeSMTP(h.Registry.AllSMTP()))
	case "imap":
		writeJSON(w, http.StatusOK, describeIMAP(h.Registry.AllIMAP()))
	default:
		writeError(w, http.StatusNotFound, "driver_category_not_found")
	}
}

func (h *AdminHandlers) GetDriver(w http.ResponseWriter, r *http.Request) {
	if h.Registry == nil {
		writeError(w, http.StatusNotFound, "driver_registry_not_configured")
		return
	}
	category := strings.TrimSpace(r.PathValue("category"))
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_driver_id")
		return
	}

	var (
		item any
		err  error
	)
	switch category {
	case "db":
		var d DBDriver
		d, err = h.Registry.GetDB(id)
		if err == nil {
			item = driverDescriptor{ID: d.ID(), DisplayName: d.DisplayName(), Category: category}
		}
	case "queue":
		var d QueueDriver
		d, err = h.Registry.GetQueue(id)
		if err == nil {
			item = driverDescriptor{ID: d.ID(), DisplayName: d.DisplayName(), Category: category}
		}
	case "file":
		var d FileDriver
		d, err = h.Registry.GetFile(id)
		if err == nil {
			item = driverDescriptor{ID: d.ID(), DisplayName: d.DisplayName(), Category: category}
		}
	case "protocol":
		var d ProtocolDriver
		d, err = h.Registry.GetProtocol(id)
		if err == nil {
			item = driverDescriptor{ID: d.ID(), DisplayName: d.DisplayName(), Category: category}
		}
	case "smtp":
		var d SMTPDriver
		d, err = h.Registry.GetSMTP(id)
		if err == nil {
			item = driverDescriptor{ID: d.ID(), DisplayName: d.DisplayName(), Category: category}
		}
	case "imap":
		var d IMAPDriver
		d, err = h.Registry.GetIMAP(id)
		if err == nil {
			item = driverDescriptor{ID: d.ID(), DisplayName: d.DisplayName(), Category: category}
		}
	default:
		writeError(w, http.StatusNotFound, "driver_category_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusNotFound, "driver_not_found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *AdminHandlers) ListPools(w http.ResponseWriter, _ *http.Request) {
	if h.Pools == nil {
		writeJSON(w, http.StatusOK, []PoolStats{})
		return
	}
	writeJSON(w, http.StatusOK, h.Pools.Stats())
}

func (h *AdminHandlers) ClosePool(w http.ResponseWriter, r *http.Request) {
	if h.Pools == nil {
		writeError(w, http.StatusNotFound, "pool_manager_not_configured")
		return
	}
	key := strings.TrimSpace(r.PathValue("key"))
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid_pool_key")
		return
	}
	if err := h.Pools.Close(parts[0], parts[1]); err != nil {
		writeError(w, http.StatusInternalServerError, "pool_close_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

type driverDescriptor struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Category    string `json:"category"`
}

func describeDB(items []DBDriver) []driverDescriptor {
	out := make([]driverDescriptor, 0, len(items))
	for _, d := range items {
		out = append(out, driverDescriptor{ID: d.ID(), DisplayName: d.DisplayName(), Category: "db"})
	}
	return out
}

func describeQueue(items []QueueDriver) []driverDescriptor {
	out := make([]driverDescriptor, 0, len(items))
	for _, d := range items {
		out = append(out, driverDescriptor{ID: d.ID(), DisplayName: d.DisplayName(), Category: "queue"})
	}
	return out
}

func describeFile(items []FileDriver) []driverDescriptor {
	out := make([]driverDescriptor, 0, len(items))
	for _, d := range items {
		out = append(out, driverDescriptor{ID: d.ID(), DisplayName: d.DisplayName(), Category: "file"})
	}
	return out
}

func describeProtocol(items []ProtocolDriver) []driverDescriptor {
	out := make([]driverDescriptor, 0, len(items))
	for _, d := range items {
		out = append(out, driverDescriptor{ID: d.ID(), DisplayName: d.DisplayName(), Category: "protocol"})
	}
	return out
}

func describeSMTP(items []SMTPDriver) []driverDescriptor {
	out := make([]driverDescriptor, 0, len(items))
	for _, d := range items {
		out = append(out, driverDescriptor{ID: d.ID(), DisplayName: d.DisplayName(), Category: "smtp"})
	}
	return out
}

func describeIMAP(items []IMAPDriver) []driverDescriptor {
	out := make([]driverDescriptor, 0, len(items))
	for _, d := range items {
		out = append(out, driverDescriptor{ID: d.ID(), DisplayName: d.DisplayName(), Category: "imap"})
	}
	return out
}
