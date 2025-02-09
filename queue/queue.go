package queue

import (
	"context"
	"log/slog"

	"github.com/stateforward/go-hsm/elements"
)

var NonBlocking = func() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}()

type Queue struct {
	empty  chan bool
	events chan []elements.Event
}

func Make() Queue {
	empty := make(chan bool, 1)
	empty <- true
	return Queue{
		empty:  empty,
		events: make(chan []elements.Event, 1),
	}
}

func (q *Queue) Push(event elements.Event) {
	var events []elements.Event
	select {
	case events = <-q.events:
		slog.Info("queue is not empty", "event", event)
	case <-q.empty:
		slog.Info("queue is empty", "event", event)
	}
	events = append(events, event)
	q.events <- events
}

func (q *Queue) Pop(ctx context.Context) (elements.Event, error) {
	var events []elements.Event
	select {
	case events = <-q.events:
		slog.Info("queue is not empty", "events", events)
	case <-ctx.Done():
		// slog.Info("queue is emptry", "events", events)
		return elements.Event{}, ctx.Err()
	}
	event := events[0]
	if len(events) == 1 {
		q.empty <- true
	} else {
		q.events <- events[1:]
	}
	return event, nil
}
