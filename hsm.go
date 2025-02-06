package hsm

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stateforward/go-hsm/embedded"
	"github.com/stateforward/go-hsm/kind"
)

var Kinds = kind.Kinds()

/******* Element *******/

type element struct {
	kind          uint64
	qualifiedName string
	id            string
}

func (element *element) Kind() uint64 {
	if element == nil {
		return 0
	}
	return element.kind
}

func (element *element) Owner() string {
	if element == nil {
		return ""
	}
	return path.Dir(element.qualifiedName)
}

func (element *element) Id() string {
	if element == nil {
		return ""
	}
	return element.id
}

func (element *element) Name() string {
	if element == nil {
		return ""
	}
	return path.Base(element.qualifiedName)
}

func (element *element) QualifiedName() string {
	if element == nil {
		return ""
	}
	return element.qualifiedName
}

/******* Model *******/

type Element = embedded.NamedElement

type Model struct {
	state
	namespace map[string]embedded.NamedElement
	elements  []RedifinableElement
}

func (model *Model) Namespace() map[string]embedded.NamedElement {
	return model.namespace
}

func (model *Model) Push(partial RedifinableElement) {
	model.elements = append(model.elements, partial)
}

type RedifinableElement = func(model *Model, stack []embedded.NamedElement) embedded.NamedElement

/******* Vertex *******/

type vertex struct {
	element
	transitions []string
}

func (vertex *vertex) Transitions() []string {
	return vertex.transitions
}

/******* State *******/

type state struct {
	vertex
	entry    string
	exit     string
	activity string
}

func (state *state) Entry() string {
	return state.entry
}

func (state *state) Activity() string {
	return state.activity
}

func (state *state) Exit() string {
	return state.exit
}

/******* Transition *******/

type paths struct {
	enter []string
	exit  []string
}

type transition struct {
	element
	source string
	target string
	guard  string
	effect string
	events []Event
	paths  map[string]paths
}

func (transition *transition) Guard() string {
	return transition.guard
}

func (transition *transition) Effect() string {
	return transition.effect
}

func (transition *transition) Events() []Event {
	return transition.events
}

func (transition *transition) Source() string {
	return transition.source
}

func (transition *transition) Target() string {
	return transition.target
}

/******* Behavior *******/

type behavior[T context.Context] struct {
	element
	method func(hsm Active[T], event *Event)
}

/******* Constraint *******/

type constraint[T context.Context] struct {
	element
	expression func(hsm Active[T], event *Event) bool
}

/******* Events *******/
type Event = embedded.Event

var noevent = Event{}

type Queue struct {
	mutex     sync.RWMutex
	events    []Event
	partition uint64
}

func (q *Queue) Len() int {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return len(q.events)
}

func (q *Queue) Pop() (Event, bool) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if len(q.events) == 0 {
		return Event{}, false
	}
	event := q.events[0]
	q.events = q.events[1:]
	if q.partition > 0 {
		q.partition--
	}
	return event, true
}

func (q *Queue) Push(event Event) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if kind.IsKind(event.Kind, kind.CompletionEvent) {
		q.events = append(q.events[q.partition:], append([]Event{event}, q.events[:q.partition]...)...)
		q.partition++
	} else {
		q.events = append(q.events, event)
	}
}

func apply(model *Model, stack []embedded.NamedElement, partials ...RedifinableElement) {
	for _, partial := range partials {
		partial(model, stack)
	}
}

func Define[T interface{ RedifinableElement | string }](nameOrRedifinableElement T, redifinableElements ...RedifinableElement) Model {
	name := "/"
	switch any(nameOrRedifinableElement).(type) {
	case string:
		name = path.Join(name, any(nameOrRedifinableElement).(string))
	case RedifinableElement:
		redifinableElements = append([]RedifinableElement{any(nameOrRedifinableElement).(RedifinableElement)}, redifinableElements...)
	}
	model := Model{
		state: state{
			vertex: vertex{element: element{kind: kind.State, qualifiedName: "/", id: name}, transitions: []string{}},
		},
		namespace: map[string]embedded.NamedElement{},
		elements:  redifinableElements,
	}

	stack := []embedded.NamedElement{&model}
	for len(model.elements) > 0 {
		elements := model.elements
		model.elements = []RedifinableElement{}
		apply(&model, stack, elements...)
	}
	return model
}

func find(stack []embedded.NamedElement, maybeKinds ...uint64) embedded.NamedElement {
	for i := len(stack) - 1; i >= 0; i-- {
		if kind.IsKind(stack[i].Kind(), maybeKinds...) {
			return stack[i]
		}
	}
	return nil
}

func get[T embedded.NamedElement](model *Model, name string) T {
	var zero T
	if name == "" {
		return zero
	}
	if element, ok := model.namespace[name]; ok {
		typed, ok := element.(T)
		if ok {
			return typed
		}
	}
	return zero
}

func State(name string, partialElements ...RedifinableElement) RedifinableElement {
	return func(graph *Model, stack []embedded.NamedElement) embedded.NamedElement {
		owner := find(stack, kind.StateMachine, kind.State)
		if owner == nil {
			panic(fmt.Errorf("state must be called within a StateMachine or State"))
		}
		element := &state{
			vertex: vertex{element: element{kind: kind.State, qualifiedName: path.Join(owner.QualifiedName(), name)}, transitions: []string{}},
		}
		graph.namespace[element.QualifiedName()] = element
		stack = append(stack, element)
		apply(graph, stack, partialElements...)
		return element
	}
}

// LCA finds the Lowest Common Ancestor between two qualified state names in a hierarchical state machine.
// It takes two qualified names 'a' and 'b' as strings and returns their closest common ancestor.
//
// For example:
// - LCA("/s/s1", "/s/s2") returns "/s"
// - LCA("/s/s1", "/s/s1/s11") returns "/s/s1"
// - LCA("/s/s1", "/s/s1") returns "/s/s1"
func LCA(a, b string) string {
	// if both are the same the lca is the parent
	if a == b {
		return path.Dir(a)
	}
	// if one is empty the lca is the other
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	// if the parents are the same the lca is the parent
	if path.Dir(a) == path.Dir(b) {
		return path.Dir(a)
	}
	// if a is an ancestor of b the lca is a
	if IsAncestor(a, b) {
		return a
	}
	// if b is an ancestor of a the lca is b
	if IsAncestor(b, a) {
		return b
	}
	// otherwise the lca is the lca of the parents
	return LCA(path.Dir(a), path.Dir(b))
}

func IsAncestor(current, target string) bool {
	current = path.Clean(current)
	target = path.Clean(target)
	if current == target || current == "." || target == "." {
		return false
	}
	if current == "/" {
		return true
	}
	parent := path.Dir(target)
	for parent != "/" {
		if parent == current {
			return true
		}
		parent = path.Dir(parent)
	}
	return false
}

func Transition[T interface{ RedifinableElement | string }](nameOrPartialElement T, partialElements ...RedifinableElement) RedifinableElement {
	name := ""
	switch any(nameOrPartialElement).(type) {
	case string:
		name = any(nameOrPartialElement).(string)
	case RedifinableElement:
		partialElements = append([]RedifinableElement{any(nameOrPartialElement).(RedifinableElement)}, partialElements...)
	}
	return func(model *Model, stack []embedded.NamedElement) embedded.NamedElement {
		owner := find(stack, kind.Vertex)
		if owner == nil {
			panic(fmt.Errorf("transition must be called within a State or StateMachine"))
		}
		if name == "" {
			name = fmt.Sprintf("transition_%d", len(model.namespace))
		}
		transition := &transition{
			events: []Event{},
			element: element{
				kind:          kind.Transition,
				qualifiedName: path.Join(owner.QualifiedName(), name),
			},
			source: ".",
			paths:  map[string]paths{},
		}
		model.namespace[transition.QualifiedName()] = transition
		stack = append(stack, transition)
		apply(model, stack, partialElements...)
		if transition.source == "." || transition.source == "" {
			transition.source = owner.QualifiedName()
		}
		sourceElement, ok := model.namespace[transition.source]
		if !ok {
			panic(fmt.Errorf("missing source \"%s\" for transition \"%s\"", transition.source, transition.QualifiedName()))
		}
		switch source := sourceElement.(type) {
		case *state:
			source.transitions = append(source.transitions, transition.QualifiedName())
		case *vertex:
			source.transitions = append(source.transitions, transition.QualifiedName())
		}
		if len(transition.events) == 0 && !kind.IsKind(sourceElement.Kind(), kind.Pseudostate) {

			// TODO: completion transition
			// qualifiedName := path.Join(transition.source, ".completion")
			// transition.events = append(transition.events, &event{
			// 	element: element{kind: kind.CompletionEvent, qualifiedName: qualifiedName},
			// })
			panic(fmt.Errorf("completion transition not implemented"))
		}
		if transition.target == transition.source {
			transition.kind = kind.Self
		} else if transition.target == "" {
			transition.kind = kind.Internal
		} else if IsAncestor(transition.source, transition.target) {
			transition.kind = kind.Local
		} else {
			transition.kind = kind.External
		}
		enter := []string{}
		entering := transition.target
		lca := LCA(transition.source, transition.target)
		for entering != lca && entering != "/" && entering != "" {
			enter = append([]string{entering}, enter...)
			entering = path.Dir(entering)
		}
		if kind.IsKind(transition.kind, kind.Self) {
			enter = append(enter, sourceElement.QualifiedName())
		}
		if kind.IsKind(sourceElement.Kind(), kind.Initial) {
			transition.paths[path.Dir(sourceElement.QualifiedName())] = paths{
				enter: enter,
				exit:  []string{sourceElement.QualifiedName()},
			}
		} else {
			model.Push(func(model *Model, stack []embedded.NamedElement) embedded.NamedElement {
				// precompute transition paths for the source state and nested states
				for qualifiedName, element := range model.namespace {
					if strings.HasPrefix(qualifiedName, transition.source) && kind.IsKind(element.Kind(), kind.Vertex, kind.StateMachine) {
						exit := []string{}
						if transition.kind != kind.Internal {
							exiting := element.QualifiedName()
							for exiting != lca && exiting != "/" && exiting != "" {
								exit = append(exit, exiting)
								exiting = path.Dir(exiting)
							}
						}
						transition.paths[element.QualifiedName()] = paths{
							enter: enter,
							exit:  exit,
						}
					}

				}
				return transition
			})
		}

		return transition
	}
}

func Source[T interface{ RedifinableElement | string }](nameOrPartialElement T) RedifinableElement {
	return func(model *Model, stack []embedded.NamedElement) embedded.NamedElement {
		owner := find(stack, kind.Transition)
		if owner == nil {
			panic(fmt.Errorf("source must be called within a Transition"))
		}
		transition := owner.(*transition)
		if transition.source != "." && transition.source != "" {
			panic(fmt.Errorf("transition \"%s\" already has a source \"%s\"", transition.QualifiedName(), transition.source))
		}
		var name string
		switch any(nameOrPartialElement).(type) {
		case string:
			name = any(nameOrPartialElement).(string)
			if !path.IsAbs(name) {
				if ancestor := find(stack, kind.State); ancestor != nil {
					name = path.Join(ancestor.QualifiedName(), name)
				}
			}
			// push a validation step to ensure the source exists after the model is built
			model.Push(func(model *Model, stack []embedded.NamedElement) embedded.NamedElement {
				if _, ok := model.namespace[name]; !ok {
					panic(fmt.Errorf("missing source %s", name))
				}
				return owner
			})
		case RedifinableElement:
			element := any(nameOrPartialElement).(RedifinableElement)(model, stack)
			if element == nil {
				panic(fmt.Errorf("source is nil"))
			}
			name = element.QualifiedName()
		}
		transition.source = name
		return owner
	}
}

func Defer(events ...uint64) RedifinableElement {
	panic("not implemented")
}

func Target[T interface{ RedifinableElement | string }](nameOrPartialElement T) RedifinableElement {
	return func(model *Model, stack []embedded.NamedElement) embedded.NamedElement {
		owner := find(stack, kind.Transition)
		if owner == nil {
			panic(fmt.Errorf("Target() must be called within a Transition"))
		}
		transition := owner.(*transition)
		if transition.target != "" {
			panic(fmt.Errorf("transition %s already has target %s", transition.QualifiedName(), transition.target))
		}
		var qualifiedName string
		switch target := any(nameOrPartialElement).(type) {
		case string:
			qualifiedName = target
			if !path.IsAbs(qualifiedName) {
				if ancestor := find(stack, kind.State); ancestor != nil {
					qualifiedName = path.Join(ancestor.QualifiedName(), qualifiedName)
				}
			}
			// push a validation step to ensure the target exists after the model is built
			model.Push(func(model *Model, stack []embedded.NamedElement) embedded.NamedElement {
				if _, exists := model.namespace[qualifiedName]; !exists {
					panic(fmt.Errorf("missing target %s for transition %s", target, transition.QualifiedName()))
				}
				return transition
			})
		case RedifinableElement:
			targetElement := target(model, stack)
			if targetElement == nil {
				panic(fmt.Errorf("target is nil"))
			}
			qualifiedName = targetElement.QualifiedName()
		}

		transition.target = qualifiedName
		return transition
	}
}

func Effect[T context.Context](fn func(hsm Active[T], event *Event), maybeName ...string) RedifinableElement {
	name := ".effect"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(model *Model, stack []embedded.NamedElement) embedded.NamedElement {
		owner := find(stack, kind.Transition)
		if owner == nil {
			slog.Error("effect must be called within a Transition")
			panic(fmt.Errorf("effect must be called within a Transition"))
		}
		behavior := &behavior[T]{
			element: element{kind: kind.Behavior, qualifiedName: path.Join(owner.QualifiedName(), name)},
			method:  fn,
		}
		model.namespace[behavior.QualifiedName()] = behavior
		owner.(*transition).effect = behavior.QualifiedName()
		return owner
	}
}

func Guard[T context.Context](fn func(hsm Active[T], event *Event) bool, maybeName ...string) RedifinableElement {
	name := ".guard"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(model *Model, stack []embedded.NamedElement) embedded.NamedElement {
		owner := find(stack, kind.Transition)
		if owner == nil {
			panic(fmt.Errorf("guard must be called within a Transition"))
		}
		constraint := &constraint[T]{
			element:    element{kind: kind.Constraint, qualifiedName: path.Join(owner.QualifiedName(), name)},
			expression: fn,
		}
		model.namespace[constraint.QualifiedName()] = constraint
		owner.(*transition).guard = constraint.QualifiedName()
		return owner
	}
}

func Initial[T interface{ string | RedifinableElement }](elementOrName T, partialElements ...RedifinableElement) RedifinableElement {
	name := ".initial"
	switch any(elementOrName).(type) {
	case string:
		name = any(elementOrName).(string)
	case RedifinableElement:
		partialElements = append([]RedifinableElement{any(elementOrName).(RedifinableElement)}, partialElements...)
	}
	return func(model *Model, stack []embedded.NamedElement) embedded.NamedElement {
		owner := find(stack, kind.State)
		if owner == nil {
			panic(fmt.Errorf("initial must be called within a State or Model"))
		}
		initial := &vertex{
			element: element{kind: kind.Initial, qualifiedName: path.Join(owner.QualifiedName(), name)},
		}
		if model.namespace[initial.QualifiedName()] != nil {
			panic(fmt.Errorf("initial \"%s\" state already exists for \"%s\"", initial.QualifiedName(), owner.QualifiedName()))
		}
		model.namespace[initial.QualifiedName()] = initial
		stack = append(stack, initial)
		transition := (Transition(Source(initial.QualifiedName()), partialElements...)(model, stack)).(*transition)
		// validation logic
		if transition.guard != "" {
			panic(fmt.Errorf("initial \"%s\" cannot have a guard", initial.QualifiedName()))
		}
		if len(transition.events) > 0 {
			panic(fmt.Errorf("initial \"%s\" cannot have triggers", initial.QualifiedName()))
		}
		if !strings.HasPrefix(transition.target, owner.QualifiedName()) {
			panic(fmt.Errorf("initial \"%s\" must target a nested state not \"%s\"", initial.QualifiedName(), transition.target))
		}
		if len(initial.transitions) > 1 {
			panic(fmt.Errorf("initial \"%s\" cannot have multiple transitions %v", initial.QualifiedName(), initial.transitions))
		}
		return transition
	}
}

func Choice[T interface{ RedifinableElement | string }](elementOrName T, partialElements ...RedifinableElement) RedifinableElement {
	name := ""
	switch any(elementOrName).(type) {
	case string:
		name = any(elementOrName).(string)
	case RedifinableElement:
		partialElements = append([]RedifinableElement{any(elementOrName).(RedifinableElement)}, partialElements...)
	}
	return func(model *Model, stack []embedded.NamedElement) embedded.NamedElement {
		owner := find(stack, kind.State, kind.Transition)
		if owner == nil {
			panic(fmt.Errorf("you must call Choice() within a State or Transition"))
		} else if kind.IsKind(owner.Kind(), kind.Transition) {
			transition := owner.(*transition)
			source := transition.source
			owner = model.namespace[source]
			if owner == nil {
				panic(fmt.Errorf("transition \"%s\" targetting \"%s\" requires a source state when using Choice()", transition.QualifiedName(), transition.target))
			} else if kind.IsKind(owner.Kind(), kind.Pseudostate) {
				// pseudostates aren't a namespace, so we need to find the containing state
				owner = find(stack, kind.State)
				if owner == nil {
					panic(fmt.Errorf("you must call Choice() within a State"))
				}
			}
		}
		if name == "" {
			name = fmt.Sprintf("choice_%d", len(model.elements))
		}
		qualifiedName := path.Join(owner.QualifiedName(), name)
		element := &vertex{
			element: element{kind: kind.Choice, qualifiedName: qualifiedName},
		}
		model.namespace[qualifiedName] = element
		stack = append(stack, element)
		apply(model, stack, partialElements...)
		if len(element.transitions) == 0 {
			panic(fmt.Errorf("you must define at least one transition for choice \"%s\"", qualifiedName))
		}
		if defaultTransition := get[embedded.Transition](model, element.transitions[len(element.transitions)-1]); defaultTransition != nil {
			if defaultTransition.Guard() != "" {
				panic(fmt.Errorf("the last transition of a choice state cannot have a guard"))
			}
		}
		return element
	}
}

func Entry[T context.Context](fn func(ctx Active[T], event *Event), maybeName ...string) RedifinableElement {
	name := ".entry"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(model *Model, stack []embedded.NamedElement) embedded.NamedElement {
		owner := find(stack, kind.State)
		if owner == nil {
			slog.Error("entry must be called within a State")
			panic(fmt.Errorf("entry must be called within a State"))
		}
		element := &behavior[T]{
			element: element{kind: kind.Behavior, qualifiedName: path.Join(owner.QualifiedName(), name)},
			method:  fn,
		}
		model.namespace[element.QualifiedName()] = element
		owner.(*state).entry = element.QualifiedName()
		return element
	}
}

func Activity[T context.Context](fn func(hsm Active[T], event *Event), maybeName ...string) RedifinableElement {
	name := ".activity"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(model *Model, stack []embedded.NamedElement) embedded.NamedElement {
		owner := find(stack, kind.State)
		if owner == nil {
			slog.Error("activity must be called within a State")
			panic(fmt.Errorf("activity must be called within a State"))
		}

		element := &behavior[T]{
			element: element{kind: kind.Concurrent, qualifiedName: path.Join(owner.QualifiedName(), name)},
			method:  fn,
		}
		model.namespace[element.QualifiedName()] = element
		owner.(*state).activity = element.QualifiedName()
		return element
	}
}

func Exit[T context.Context](fn func(hsm Active[T], event *Event), maybeName ...string) RedifinableElement {
	name := ".exit"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(model *Model, stack []embedded.NamedElement) embedded.NamedElement {
		owner := find(stack, kind.State)
		if owner == nil {
			slog.Error("exit must be called within a State")
			panic(fmt.Errorf("exit must be called within a State"))
		}

		element := &behavior[T]{
			element: element{kind: kind.Behavior, qualifiedName: path.Join(owner.QualifiedName(), name)},
			method:  fn,
		}
		model.namespace[element.QualifiedName()] = element
		owner.(*state).exit = element.QualifiedName()
		return element
	}
}

func Trigger[T interface{ string | *Event | Event }](events ...T) RedifinableElement {
	return func(model *Model, stack []embedded.NamedElement) embedded.NamedElement {
		owner := find(stack, kind.Transition)
		if owner == nil {
			panic(fmt.Errorf("trigger must be called within a Transition"))
		}
		transition := owner.(*transition)
		for _, eventOrName := range events {
			switch any(eventOrName).(type) {
			case string:
				name := any(eventOrName).(string)
				transition.events = append(transition.events, Event{
					Kind: kind.Event,
					Name: name,
				})
			case Event:
				transition.events = append(transition.events, any(eventOrName).(Event))
			case *Event:
				event := any(eventOrName).(*Event)
				transition.events = append(transition.events, *event)
			}
		}
		return owner
	}
}

func After[T context.Context](expr func(hsm Active[T]) time.Duration, maybeName ...string) RedifinableElement {
	name := ".after"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(builder *Model, stack []embedded.NamedElement) embedded.NamedElement {
		owner := find(stack, kind.Transition)
		if owner == nil {
			panic(fmt.Errorf("after must be called within a Transition"))
		}
		qualifiedName := path.Join(owner.QualifiedName(), strconv.Itoa(len(owner.(*transition).events)), name)
		owner.(*transition).events = append(owner.(*transition).events, Event{
			Kind: kind.TimeEvent,
			Name: qualifiedName,
			Data: expr,
		})
		return owner
	}
}

// var pool = sync.Pool{
// 	New: func() any {
// 		return &event{
// 			kind: kind.Event,
// 			name: "",
// 			id:   "",
// 			data: nil,
// 		}
// 	},
// }

// func NewEvent(name string, data any, maybeId ...string) *event {
// 	event := &event{
// 		kind: kind.Event,
// 		name: name,
// 		data: data,
// 	}
// 	if len(maybeId) > 0 {
// 		event.id = maybeId[0]
// 	}
// 	return event
// }

func Final(name string) RedifinableElement {
	return func(builder *Model, stack []embedded.NamedElement) embedded.NamedElement {
		panic("not implemented")
	}
}

type HSM interface {
	context.Context
	Element
	State() string
	Terminate()
	Dispatch(event Event)
}

type subcontext = context.Context

type hsm[T context.Context] struct {
	behavior[T]
	state      embedded.NamedElement
	model      *Model
	active     map[string]*Active[T]
	queue      Queue
	processing atomic.Bool
	Context    T
	trace      Trace
}

type Active[T context.Context] struct {
	subcontext
	*hsm[T]
	cancel  context.CancelFunc
	channel chan struct{}
}

func (active Active[T]) Dispatch(event Event) {
	if active.cancel != nil {
		go active.dispatch(event)
	} else {
		active.dispatch(event)
	}
}

type Trace func(hsm HSM, step string, data ...any) (context.Context, func(...any))

func WithTrace[T context.Context](trace Trace) Option[T] {
	return func(hsm *hsm[T]) {
		hsm.trace = trace
	}
}

func WithId[T context.Context](id string) Option[T] {
	return func(hsm *hsm[T]) {
		hsm.id = id
	}
}

type Option[T context.Context] func(hsm *hsm[T])

type key[T any] struct{}

var Keys = struct {
	All key[*sync.Map]
	HSM key[HSM]
}{
	All: key[*sync.Map]{},
	HSM: key[HSM]{},
}

func noop() {}

func New[T context.Context](ctx T, model *Model, options ...Option[T]) Active[T] {
	hsm := &hsm[T]{
		behavior: behavior[T]{
			element: element{
				kind:          kind.StateMachine,
				qualifiedName: model.QualifiedName(),
			},
		},
		model:   model,
		active:  map[string]*Active[T]{},
		Context: ctx,
		queue:   Queue{},
	}
	for _, option := range options {
		option(hsm)
	}
	all, ok := ctx.Value(Keys.All).(*sync.Map)
	if !ok {
		all = &sync.Map{}
	}
	active := &Active[T]{
		hsm: hsm,
	}
	active.subcontext = context.WithValue(context.WithValue(context.Background(), Keys.All, all), Keys.HSM, active)
	all.Store(hsm, active)
	hsm.method = func(_ Active[T], event *Event) {
		active.processing.Store(true)
		defer active.processing.Store(false)
		active.state = active.initial(&model.state, event)
	}
	active.execute(&hsm.behavior, nil)
	return *active
}

func (active *Active[T]) State() string {
	if active == nil {
		return ""
	}
	if active.state == nil {
		return ""
	}
	return active.state.QualifiedName()
}

func (active *Active[T]) Terminate() {
	if active == nil {
		return
	}
	if active.trace != nil {
		ctx, end := active.trace(active, "Terminate", active.state)
		active = &Active[T]{
			subcontext: ctx,
			hsm:        active.hsm,
			cancel:     active.cancel,
		}
		defer end()
	}
	var ok bool
	for active.state != nil {
		active.exit(active.state, nil)
		active.state, ok = active.model.namespace[active.state.Owner()]
		if !ok {
			break
		}
	}
	all, ok := active.Value(Keys.All).(*sync.Map)
	if !ok {
		return
	}
	all.Delete(active.hsm)
}

func (active *Active[T]) activate(id string) *Active[T] {
	current, ok := active.active[id]
	if !ok {
		current = &Active[T]{
			channel: make(chan struct{}, 1),
		}
		active.active[id] = current
	}
	current.subcontext, current.cancel = context.WithCancel(active)
	return current
}

func (active *Active[T]) enter(element embedded.NamedElement, event *Event, defaultEntry bool) embedded.NamedElement {
	if active == nil {
		return nil
	}
	if active.trace != nil {
		ctx, end := active.trace(active, "enter", element)
		defer end()
		active = &Active[T]{
			subcontext: ctx,
			hsm:        active.hsm,
			cancel:     active.cancel,
		}
	}
	switch element.Kind() {
	case kind.State:
		state := element.(*state)
		if entry := get[*behavior[T]](active.model, state.entry); entry != nil {
			active.execute(entry, event)
		}
		activity := get[*behavior[T]](active.model, state.activity)
		if activity != nil {
			active.execute(activity, event)
		}
		for _, qualifiedName := range state.transitions {
			if element := get[*transition](active.model, qualifiedName); element != nil {
				for _, event := range element.Events() {
					switch event.Kind {
					case kind.TimeEvent:
						ctx := active.activate(event.Id)
						go func(ctx *Active[T], event Event) {
							duration := event.Data.(func(hsm Active[T]) time.Duration)(
								Active[T]{
									subcontext: ctx,
									hsm:        active.hsm,
									cancel:     noop,
								},
							)
							timer := time.NewTimer(duration)
							defer timer.Stop()
							select {
							case <-ctx.Done():
								break
							case <-active.Context.Done():
								break
							case <-timer.C:
								timer.Stop()
								active.dispatch(event)
								return
							}
						}(ctx, event)
					}
				}
			}
		}
		if !defaultEntry {
			return element
		}
		return active.initial(element, event)
	case kind.Choice:
		for _, qualifiedName := range element.(*vertex).transitions {
			if transition := get[*transition](active.model, qualifiedName); transition != nil {
				if constraint := get[*constraint[T]](active.model, transition.Guard()); constraint != nil {
					if !active.evaluate(constraint, event) {
						continue
					}
				}
				return active.transition(element, transition, event)
			}
		}
	}
	return nil
}

func (active *Active[T]) initial(element embedded.NamedElement, event *Event) embedded.NamedElement {
	if active == nil || element == nil {
		return nil
	}
	if active.trace != nil {
		ctx, end := active.trace(active, "initial", element)
		defer end()
		active = &Active[T]{
			subcontext: ctx,
			hsm:        active.hsm,
			cancel:     active.cancel,
		}
	}
	var qualifiedName string
	if element.QualifiedName() == "/" {
		qualifiedName = "/.initial"
	} else {
		qualifiedName = element.QualifiedName() + "/.initial"
	}
	if initial := get[*vertex](active.model, qualifiedName); initial != nil {
		if len(initial.transitions) > 0 {
			if transition := get[*transition](active.model, initial.transitions[0]); transition != nil {
				return active.transition(element, transition, event)
			}
		}
	}
	return element
}

func (active *Active[T]) exit(element embedded.NamedElement, event *Event) {
	if active == nil || element == nil {
		return
	}
	if active.trace != nil {
		ctx, end := active.trace(active, "exit", element)
		defer end()
		active = &Active[T]{
			subcontext: ctx,
			hsm:        active.hsm,
			cancel:     active.cancel,
		}
	}
	if state, ok := element.(*state); ok {
		for _, qualifiedName := range state.transitions {
			if element := get[*transition](active.model, qualifiedName); element != nil {
				for _, event := range element.Events() {
					switch event.Kind {
					case kind.TimeEvent:
						active, ok := active.active[event.Id]
						if ok {
							active.cancel()
						}
					}
				}
			}
		}
		if activity := get[*behavior[T]](active.model, state.activity); activity != nil {
			active.terminate(activity)
		}
		if exit := get[*behavior[T]](active.model, state.exit); exit != nil {
			active.execute(exit, event)
		}
	}

}

func (active *Active[T]) execute(element *behavior[T], event *Event) {
	if active == nil || element == nil {
		return
	}
	var end func(...any)
	if active.trace != nil {
		ctx, end := active.trace(active, "execute", element, event)
		defer end()
		active = &Active[T]{
			subcontext: ctx,
			hsm:        active.hsm,
			cancel:     active.cancel,
		}
	}
	switch element.Kind() {
	case kind.Concurrent:
		ctx := active.activate(element.QualifiedName())
		go func(ctx *Active[T], end func(...any)) {
			if end != nil {
				defer end()
			}
			element.method(Active[T]{
				subcontext: ctx,
				hsm:        active.hsm,
				cancel:     ctx.cancel,
			}, event)
			ctx.channel <- struct{}{}
		}(ctx, end)
	default:
		element.method(Active[T]{
			subcontext: active,
			hsm:        active.hsm,
			cancel:     noop,
		}, event)

	}

}

func (active *Active[T]) evaluate(guard *constraint[T], event *Event) bool {
	if active == nil || guard == nil || guard.expression == nil {
		return true
	}
	if active.trace != nil {
		ctx, end := active.trace(active, "evaluate", guard, event)
		defer end()
		active = &Active[T]{
			subcontext: ctx,
			hsm:        active.hsm,
			cancel:     active.cancel,
		}
	}
	return guard.expression(
		Active[T]{
			subcontext: active,
			hsm:        active.hsm,
			cancel:     noop,
		},
		event,
	)
}

func (active *Active[T]) transition(current embedded.NamedElement, transition *transition, event *Event) embedded.NamedElement {
	if active == nil {
		return nil
	}
	if active.trace != nil {
		ctx, end := active.trace(active, "transition", current, transition, event)
		defer end()
		active = &Active[T]{
			subcontext: ctx,
			hsm:        active.hsm,
			cancel:     active.cancel,
		}
	}
	path, ok := transition.paths[current.QualifiedName()]
	if !ok {
		return nil
	}
	for _, exiting := range path.exit {
		current, ok = active.model.namespace[exiting]
		if !ok {
			return nil
		}
		active.exit(current, event)
	}
	if effect := get[*behavior[T]](active.model, transition.effect); effect != nil {
		active.execute(effect, event)
	}
	if kind.IsKind(transition.kind, kind.Internal) {
		return current
	}
	for _, entering := range path.enter {
		next, ok := active.model.namespace[entering]
		if !ok {
			return nil
		}
		defaultEntry := entering == transition.target
		current = active.enter(next, event, defaultEntry)
		if defaultEntry {
			return current
		}
	}
	current, ok = active.model.namespace[transition.target]
	if !ok {
		return nil
	}
	return current
}

func (active *Active[T]) terminate(behavior *behavior[T]) {
	if active == nil || behavior == nil {
		return
	}
	if active.trace != nil {
		ctx, end := active.trace(active, "terminate", behavior)
		defer end()
		active = &Active[T]{
			subcontext: ctx,
			hsm:        active.hsm,
			cancel:     active.cancel,
		}
	}
	active, ok := active.active[behavior.QualifiedName()]
	if !ok {
		return
	}
	active.cancel()
	<-active.channel

}

func (active *Active[T]) enabled(source embedded.Vertex, event *Event) *transition {
	if active == nil {
		return nil
	}
	for _, transitionQualifiedName := range source.Transitions() {
		transition := get[*transition](active.model, transitionQualifiedName)
		if transition == nil {
			continue
		}
		for _, evt := range transition.Events() {
			if matched, err := path.Match(evt.Name, event.Name); err != nil || !matched {
				continue
			}
			if guard := get[*constraint[T]](active.model, transition.Guard()); guard != nil {
				if !active.evaluate(guard, event) {
					continue
				}
			}
			return transition
		}
	}
	return nil
}

func (active *Active[T]) process(event Event) {
	if active.processing.Load() {
		return
	}
	active.processing.Store(true)
	defer active.processing.Store(false)
	ok := true
	for ok {
		state := active.state.QualifiedName()
		for state != "/" {
			source := get[embedded.Vertex](active.model, state)
			if source == nil {
				break
			}
			if transition := active.enabled(source, &event); transition != nil {
				active.state = active.transition(active.state, transition, &event)
				break
			}
			state = source.Owner()
		}
		event, ok = active.queue.Pop()
	}
}

func (active *Active[T]) dispatch(event Event) {
	if active == nil {
		return
	}
	if active.state == nil {
		return
	}
	if event.Kind == 0 {
		event.Kind = kind.Event
	}
	if active.trace != nil {
		ctx, end := active.trace(active, "Dispatch", event)
		defer end()
		active = &Active[T]{
			subcontext: ctx,
			hsm:        active.hsm,
			cancel:     active.cancel,
		}
	}
	if active.processing.Load() {
		active.queue.Push(event)
		return
	}
	active.process(event)
}

func Dispatch[T context.Context](ctx T, event Event) bool {
	if hsm, ok := FromContext(ctx); ok {
		hsm.Dispatch(event)
		return true
	}
	return false
}

func DispatchAll[T context.Context](ctx T, event *Event) bool {
	all, ok := ctx.Value(Keys.All).(*sync.Map)
	if !ok {
		return false
	}
	all.Range(func(_ any, value any) bool {
		maybeActive, ok := value.(interface{ Dispatch(*Event) })
		if !ok {
			return true
		}
		maybeActive.Dispatch(event)
		return true
	})
	return true
}

func FromContext[T context.Context](ctx T) (HSM, bool) {
	switch any(ctx).(type) {
	case HSM:
		return any(ctx).(HSM), true
	default:
		hsm, ok := ctx.Value(Keys.HSM).(HSM)
		if ok {
			return hsm, true
		}
	}
	return nil, false
}
