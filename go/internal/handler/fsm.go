package handler

import "sync"

type pendingState struct {
	Kind string
	Data map[string]string
}

type fsmStore struct {
	mu     sync.Mutex
	states map[int64]*pendingState
}

func newFSM() *fsmStore {
	return &fsmStore{states: make(map[int64]*pendingState)}
}

func (f *fsmStore) Set(userID int64, kind string, data map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states[userID] = &pendingState{Kind: kind, Data: data}
}

func (f *fsmStore) Get(userID int64) (*pendingState, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.states[userID]
	return s, ok
}

func (f *fsmStore) Clear(userID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.states, userID)
}
