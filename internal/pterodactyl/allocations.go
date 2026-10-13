package pterodactyl

import (
	"context"
	"fmt"
)

// AllocationManager belongs to the worker's Application API, never the browser.
// Limits are supplied from the purchased database snapshot, not customer inputs.
type AdditionalAllocationRequest struct {
	ServerID               int
	PrimaryAllocationID    int
	AdditionalAllocationID int
	Limits                 Limits
}

type AllocationManager interface {
	ServerAllocations(context.Context, int) ([]int, error)
	AssignAllocation(context.Context, AdditionalAllocationRequest) error
}

func (a *ApplicationHTTP) ServerAllocations(ctx context.Context, id int) ([]int, error) {
	if id <= 0 {
		return nil, ErrInvalid
	}
	var response struct {
		Attributes struct {
			ID            int `json:"id"`
			Relationships struct {
				Allocations struct {
					Data []struct {
						Attributes struct {
							ID int `json:"id"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"allocations"`
			} `json:"relationships"`
		} `json:"attributes"`
	}
	if err := a.request(ctx, "GET", fmt.Sprintf("/api/application/servers/%d?include=allocations", id), nil, &response); err != nil {
		return nil, err
	}
	if response.Attributes.ID != id {
		return nil, ErrInvalid
	}
	ids := []int{}
	for _, entry := range response.Attributes.Relationships.Allocations.Data {
		if entry.Attributes.ID <= 0 {
			return nil, ErrInvalid
		}
		ids = append(ids, entry.Attributes.ID)
	}
	if len(ids) == 0 {
		return nil, ErrInvalid
	}
	return ids, nil
}
func (a *ApplicationHTTP) AssignAllocation(ctx context.Context, r AdditionalAllocationRequest) error {
	id, allocation, limits := r.ServerID, r.AdditionalAllocationID, r.Limits
	if id <= 0 || allocation <= 0 || r.PrimaryAllocationID <= 0 || r.PrimaryAllocationID == allocation || limits.RAMMB <= 0 || limits.DiskMB <= 0 || limits.CPU <= 0 || limits.Backups < 0 || limits.Databases < 0 {
		return ErrInvalid
	}
	input := map[string]any{"allocation": r.PrimaryAllocationID, "add_allocations": []int{allocation}, "limits": map[string]int{"memory": limits.RAMMB, "disk": limits.DiskMB, "cpu": limits.CPU, "swap": 0, "io": 500}, "feature_limits": map[string]int{"databases": limits.Databases, "backups": limits.Backups, "allocations": 0}}
	return a.request(ctx, "PATCH", fmt.Sprintf("/api/application/servers/%d/build", id), input, nil)
}
func (m *Mock) ServerAllocations(ctx context.Context, id int) ([]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	remote, ok := m.servers[id]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]int{remote.Request.AllocationID}, m.extraAllocations[id]...), nil
}
func (m *Mock) AssignAllocation(ctx context.Context, r AdditionalAllocationRequest) error {
	id, allocation, limits := r.ServerID, r.AdditionalAllocationID, r.Limits
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	remote, ok := m.servers[id]
	if !ok {
		return ErrNotFound
	}
	if remote.Request.Limits != limits || allocation <= 0 || remote.Request.AllocationID != r.PrimaryAllocationID || r.PrimaryAllocationID == allocation {
		return ErrInvalid
	}
	for serverID, allocations := range m.extraAllocations {
		for _, existing := range allocations {
			if existing == allocation {
				if serverID == id {
					return nil
				}
				return ErrConflict
			}
		}
	}
	for serverID, s := range m.servers {
		if s.Request.AllocationID == allocation {
			if serverID == id {
				return nil
			}
			return ErrConflict
		}
	}
	m.extraAllocations[id] = append(m.extraAllocations[id], allocation)
	return nil
}

// NetworkReader uses only the container-scoped Client API for ownership checks.
type NetworkReader interface {
	NetworkAllocations(context.Context, string) ([]int, error)
}

func (a *HTTPServerClient) NetworkAllocations(ctx context.Context, id string) ([]int, error) {
	if !safeIdentifier(id) {
		return nil, ErrInvalid
	}
	var response struct {
		Data []struct {
			Attributes struct {
				ID int `json:"id"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := a.request(ctx, "GET", prefix(id)+"/network/allocations", nil, &response); err != nil {
		return nil, err
	}
	ids := []int{}
	for _, entry := range response.Data {
		if entry.Attributes.ID <= 0 {
			return nil, ErrInvalid
		}
		ids = append(ids, entry.Attributes.ID)
	}
	if len(ids) == 0 {
		return nil, ErrInvalid
	}
	return ids, nil
}
func (m *Mock) NetworkAllocations(ctx context.Context, id string) ([]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for serverID, remote := range m.servers {
		if remote.Identifier == id {
			return append([]int{remote.Request.AllocationID}, m.extraAllocations[serverID]...), nil
		}
	}
	return nil, ErrNotFound
}
