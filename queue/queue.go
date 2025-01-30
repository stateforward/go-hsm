package queue

import (
	"sync/atomic"

	"github.com/stateforward/go-hsm/embedded"
	"github.com/stateforward/go-hsm/kinds"
)

type Queue struct {
	events    atomic.Pointer[[]embedded.Event]
	partition int
}

func (q *Queue) Len() int {
	return len(*q.events.Load())
}

func (q *Queue) Pop() embedded.Event {
	events := *q.events.Load()
	if len(events) == 0 {
		return nil
	}
	event := events[0]
	events = events[1:]
	q.events.Store(&events)
	return event
}

func (q *Queue) Push(event embedded.Event) {
	events := *q.events.Load()
	if kinds.IsKind(event.Kind(), kinds.CompletionEvent) {
		events = append(events[q.partition:], append([]embedded.Event{event}, events[:q.partition]...)...)
		q.partition++
	} else {
		events = append(events, event)
	}
	q.events.Store(&events)
}

func New(maybeSize ...int) *Queue {
	var events []embedded.Event
	if len(maybeSize) > 0 {
		events = make([]embedded.Event, maybeSize[0])
	}
	q := &Queue{}
	q.events.Store(&events)
	return q
}
