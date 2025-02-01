package embedded

import "context"

type Type interface{}

type Element interface {
	Kind() uint64
	Owner() string
	QualifiedName() string
	Name() string
	Id() string
}

type Model interface {
	Element
	Namespace() map[string]Element
}

type Transition interface {
	Element
	Source() string
	Target() string
	Guard() string
	Effect() string
	Events() []Event
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

type HSM interface {
	context.Context
	Element
	State() string
	Terminate()
	Dispatch(event Event)
	DispatchAll(event Event)
}
