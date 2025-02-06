package elements

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

type Event struct {
	Kind uint64
	Name string
	Id   string
	Data any
}

func (e Event) WithData(data any) Event {
	return Event{
		Kind: e.Kind,
		Name: e.Name,
		Id:   e.Id,
		Data: data,
	}
}

type Constraint interface {
	NamedElement
	Expression() any
}

type Behavior interface {
	NamedElement
	Action() any
}
