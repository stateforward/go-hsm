package elements

import (
	"context"
)

type Type interface{}

type Element interface {
	Kind() uint64
	Owner() string
	QualifiedName() string
	Name() string
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
	Entry() string
	Activity() string
	Exit() string
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

type StateMachine interface {
	Element
	State() string
	Dispatch(event Event) bool
	DispatchAll(event Event)
	Terminate()
}

type Context interface {
	StateMachine
	context.Context
}
