package gosocketio

import (
	"errors"
	"sync"
)

var (
	ErrorWaiterNotFound = errors.New("waiter not found")
)

type ackProcessor struct {
	counter      int
	counterMutex sync.Mutex

	resultWaiters      map[int]chan string
	resultWaitersMutex sync.RWMutex
}

// getNextId of ack call
func (a *ackProcessor) getNextId() int {
	a.counterMutex.Lock()
	defer a.counterMutex.Unlock()

	a.counter++
	return a.counter
}

// addWaiter should be called just before the ack function called,
// waiter is needed to wait and receive response to ack call
func (a *ackProcessor) addWaiter(id int, w chan string) {
	a.resultWaitersMutex.Lock()
	a.resultWaiters[id] = w
	a.resultWaitersMutex.Unlock()
}

// removeWaiter that is unnecessary anymore
func (a *ackProcessor) removeWaiter(id int) {
	a.resultWaitersMutex.Lock()
	delete(a.resultWaiters, id)
	a.resultWaitersMutex.Unlock()
}

// getWaiter checks if waiter with given ack id exists, and returns it
func (a *ackProcessor) getWaiter(id int) (chan string, error) {
	a.resultWaitersMutex.RLock()
	defer a.resultWaitersMutex.RUnlock()

	if waiter, ok := a.resultWaiters[id]; ok {
		return waiter, nil
	}

	return nil, ErrorWaiterNotFound
}
