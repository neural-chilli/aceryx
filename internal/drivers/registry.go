package drivers

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"
)

type LicenceChecker interface {
	HasFeature(feature string) bool
}

type DriverRegistry struct {
	mu        sync.RWMutex
	databases map[string]DBDriver
	queues    map[string]QueueDriver
	files     map[string]FileDriver
	protocols map[string]ProtocolDriver
	smtps     map[string]SMTPDriver
	imaps     map[string]IMAPDriver
	licence   LicenceChecker
}

func NewDriverRegistry() *DriverRegistry {
	return &DriverRegistry{
		databases: map[string]DBDriver{},
		queues:    map[string]QueueDriver{},
		files:     map[string]FileDriver{},
		protocols: map[string]ProtocolDriver{},
		smtps:     map[string]SMTPDriver{},
		imaps:     map[string]IMAPDriver{},
	}
}

func (r *DriverRegistry) SetLicenceChecker(checker LicenceChecker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.licence = checker
}

func (r *DriverRegistry) RegisterDB(driver DBDriver) {
	if driver == nil || driver.ID() == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.databases[driver.ID()] = driver
}

func (r *DriverRegistry) RegisterQueue(driver QueueDriver) {
	if driver == nil || driver.ID() == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queues[driver.ID()] = driver
}

func (r *DriverRegistry) RegisterFile(driver FileDriver) {
	if driver == nil || driver.ID() == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.files[driver.ID()] = driver
}

func (r *DriverRegistry) RegisterProtocol(driver ProtocolDriver, licence ...string) {
	if driver == nil || driver.ID() == "" {
		return
	}
	if len(licence) > 0 {
		feature := licence[0]
		r.mu.RLock()
		checker := r.licence
		r.mu.RUnlock()
		if checker == nil || !checker.HasFeature(feature) {
			slog.Warn("protocol driver skipped due to missing licence", "driver_id", driver.ID(), "feature", feature)
			return
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.protocols[driver.ID()] = driver
}

func (r *DriverRegistry) RegisterSMTP(driver SMTPDriver) {
	if driver == nil || driver.ID() == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.smtps[driver.ID()] = driver
}

func (r *DriverRegistry) RegisterIMAP(driver IMAPDriver) {
	if driver == nil || driver.ID() == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.imaps[driver.ID()] = driver
}

func (r *DriverRegistry) GetDB(id string) (DBDriver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.databases[id]
	if !ok {
		return nil, fmt.Errorf("database driver not found: %s", id)
	}
	return d, nil
}

func (r *DriverRegistry) GetQueue(id string) (QueueDriver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.queues[id]
	if !ok {
		return nil, fmt.Errorf("queue driver not found: %s", id)
	}
	return d, nil
}

func (r *DriverRegistry) GetFile(id string) (FileDriver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.files[id]
	if !ok {
		return nil, fmt.Errorf("file driver not found: %s", id)
	}
	return d, nil
}

func (r *DriverRegistry) GetProtocol(id string) (ProtocolDriver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.protocols[id]
	if !ok {
		return nil, fmt.Errorf("protocol driver not found: %s", id)
	}
	return d, nil
}

func (r *DriverRegistry) GetSMTP(id string) (SMTPDriver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.smtps[id]
	if !ok {
		return nil, fmt.Errorf("smtp driver not found: %s", id)
	}
	return d, nil
}

func (r *DriverRegistry) GetIMAP(id string) (IMAPDriver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.imaps[id]
	if !ok {
		return nil, fmt.Errorf("imap driver not found: %s", id)
	}
	return d, nil
}

func (r *DriverRegistry) AllDB() []DBDriver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortByIDDB(r.databases)
}

func (r *DriverRegistry) AllQueues() []QueueDriver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortByIDQueue(r.queues)
}

func (r *DriverRegistry) AllFiles() []FileDriver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortByIDFile(r.files)
}

func (r *DriverRegistry) AllProtocols() []ProtocolDriver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortByIDProtocol(r.protocols)
}

func (r *DriverRegistry) AllSMTP() []SMTPDriver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortByIDSMTP(r.smtps)
}

func (r *DriverRegistry) AllIMAP() []IMAPDriver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortByIDIMAP(r.imaps)
}

func sortByIDDB(items map[string]DBDriver) []DBDriver {
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]DBDriver, 0, len(ids))
	for _, id := range ids {
		out = append(out, items[id])
	}
	return out
}

func sortByIDQueue(items map[string]QueueDriver) []QueueDriver {
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]QueueDriver, 0, len(ids))
	for _, id := range ids {
		out = append(out, items[id])
	}
	return out
}

func sortByIDFile(items map[string]FileDriver) []FileDriver {
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]FileDriver, 0, len(ids))
	for _, id := range ids {
		out = append(out, items[id])
	}
	return out
}

func sortByIDProtocol(items map[string]ProtocolDriver) []ProtocolDriver {
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]ProtocolDriver, 0, len(ids))
	for _, id := range ids {
		out = append(out, items[id])
	}
	return out
}

func sortByIDSMTP(items map[string]SMTPDriver) []SMTPDriver {
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]SMTPDriver, 0, len(ids))
	for _, id := range ids {
		out = append(out, items[id])
	}
	return out
}

func sortByIDIMAP(items map[string]IMAPDriver) []IMAPDriver {
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]IMAPDriver, 0, len(ids))
	for _, id := range ids {
		out = append(out, items[id])
	}
	return out
}
