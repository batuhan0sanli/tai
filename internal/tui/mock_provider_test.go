package tui

import (
	"sync"
	"sync/atomic"
)

// mockProvider is a configurable AIProvider used by the TUI tests. It records
// every call and returns the next queued response. If no responses are queued
// it falls back to defaultResp / defaultErr.
type mockProvider struct {
	mu          sync.Mutex
	calls       []string
	responses   []mockResponse
	defaultResp string
	defaultErr  error
	callCount   int32
}

type mockResponse struct {
	out string
	err error
}

func (m *mockProvider) GenerateCommand(prompt string) (string, error) {
	atomic.AddInt32(&m.callCount, 1)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, prompt)
	if len(m.responses) > 0 {
		r := m.responses[0]
		m.responses = m.responses[1:]
		return r.out, r.err
	}
	return m.defaultResp, m.defaultErr
}

func (m *mockProvider) callsSnapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.calls))
	copy(out, m.calls)
	return out
}

func (m *mockProvider) callsMade() int { return int(atomic.LoadInt32(&m.callCount)) }
