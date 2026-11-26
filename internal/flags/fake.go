package flags

import (
	"context"
	"sync"
)

// Memory is an in-memory Provider for tests. A flag it wasn't given is not found.
type Memory struct {
	_ struct{}
	// Statuses are keyed "project/flag/environment".
	Statuses map[string]Status
	// Asked records every request, in order.
	Asked []Request
	mu    sync.Mutex
}

var _ Provider = (*Memory)(nil)

// Status implements Provider.
func (m *Memory) Status(_ context.Context, request *Request) Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Asked = append(m.Asked, *request)

	if status, ok := m.Statuses[request.Project+"/"+request.Flag+"/"+request.Environment]; ok {
		return status
	}

	status := Empty(StateNotFound, "")
	status.Project, status.Environment = request.Project, request.Environment

	return status
}
