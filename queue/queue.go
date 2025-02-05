package queue

import (
	"sync"

	"github.com/stateforward/go-hsm/embedded"
	"github.com/stateforward/go-hsm/kind"
)

type Queue struct {
	mutex     sync.RWMutex
	events    []embedded.Event
	partition uint64
}

func (q *Queue) Len() int {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return len(q.events)
}

func (q *Queue) Pop() embedded.Event {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if len(q.events) == 0 {
		return nil
	}
	event := q.events[0]
	q.events = q.events[1:]
	if q.partition > 0 {
		q.partition--
	}
	return event
}

func (q *Queue) Push(event embedded.Event) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if kind.IsKind(event.Kind(), kind.CompletionEvent) {
		q.events = append(q.events[q.partition:], append([]embedded.Event{event}, q.events[:q.partition]...)...)
		q.partition++
	} else {
		q.events = append(q.events, event)
	}
}

func New(maybeSize ...int) *Queue {
	var events []embedded.Event
	if len(maybeSize) > 0 {
		events = make([]embedded.Event, maybeSize[0])
	}
	q := &Queue{}
	q.events = events
	return q
}
