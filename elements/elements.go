package elements

import (
	"context"
)

type Type interface{}

type Element interface {
	Kind() uint64
	Owner() string
	QualifiedName() string
}

type Model interface {
	Element
	Elements() map[string]Element
}

type Transition interface {
	Element
	Source() string
	Target() string
	Guard() string
	Effect() string
	Events() map[string]Event
}

type Vertex interface {
	Element
	Transitions() []string
}

type State interface {
	Vertex
	Initial() string
}

type Event interface {
	Element
	Name() string
	Data() any
}

type Constraint interface {
	Element
	Expression() any
}

type Behavior interface {
	Element
	Action() any
}

type Context[T context.Context] interface {
	Element
	context.Context
	Dispatch(event Event) bool
	DispatchAll(event Event) chan struct{}
	Storage() T
}
