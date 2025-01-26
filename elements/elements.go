package elements

import (
	"context"
	"hash/fnv"
	"path"
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

type StateMachine interface {
	Element
	context.Context
	Dispatch(event Event) bool
	DispatchAll(event Event) chan struct{}
	Storage() any
}

type element struct {
	kind          uint64
	qualifiedName string
	id            uint64
}

func (element *element) Kind() uint64 {
	return element.kind
}

func (element *element) Id() uint64 {
	return element.id
}

func (element *element) QualifiedName() string {
	return element.qualifiedName
}

func (element *element) Owner() string {
	return path.Dir(element.qualifiedName)
}

func (element *element) Name() string {
	return path.Base(element.qualifiedName)
}

func id(qualifiedName string) uint64 {
	id := fnv.New64()
	id.Write([]byte(qualifiedName))
	return id.Sum64()
}

func MakeElement(qualifiedName string, maybeKind ...uint64) element {
	kind := uint64(0)
	if len(maybeKind) > 0 {
		kind = maybeKind[0]
	}
	return element{qualifiedName: qualifiedName, id: id(qualifiedName), kind: kind}
}
