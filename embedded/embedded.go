package embedded

import (
	"context"
)

type Type interface{}

type Element interface {
	Kind() uint64
	Id() string
}

type NamedElement interface {
	Element
	Owner() string
	QualifiedName() string
	Name() string
}

type Model interface {
	NamedElement
	Namespace() map[string]NamedElement
}

type Transition interface {
	NamedElement
	Source() string
	Target() string
	Guard() string
	Effect() string
	Events() []Event
}

type Vertex interface {
	NamedElement
	Transitions() []string
}

type State interface {
	Vertex
	Entry() string
	Activity() string
	Exit() string
}

type Event interface {
	Kind() uint64
	Name() string
	Data() any
	Id() string
	Clone(data any, maybeId ...string) Event
}

type Constraint interface {
	NamedElement
	Expression() any
}

type Behavior interface {
	NamedElement
	Action() any
}

type Active interface {
	context.Context
	NamedElement
	State() string
	Terminate()
	Dispatch(event Event)
	DispatchAll(event Event)
}
