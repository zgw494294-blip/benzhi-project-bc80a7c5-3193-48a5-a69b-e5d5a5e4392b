package application

import "sync"

type batchSerial struct {
	mu    sync.Mutex
	locks sync.Map
}
type lockEntry struct {
	mu   sync.Mutex
	refs int
}

func (s *batchSerial) execute(batchID string, fn func() error) (err error) {
	s.mu.Lock()
	value, _ := s.locks.LoadOrStore(batchID, &lockEntry{})
	entry := value.(*lockEntry)
	entry.refs++
	s.mu.Unlock()

	entry.mu.Lock()
	defer func() {
		entry.mu.Unlock()
		s.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			s.locks.Delete(batchID)
		}
		s.mu.Unlock()
	}()
	err = fn()
	return err
}
