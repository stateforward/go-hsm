package hsm

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stateforward/go-hsm/elements"
	"github.com/stateforward/go-hsm/kind"
)

var Kinds = kind.Kinds()

type Active interface {
	Element
	context.Context
	State() string
	Dispatch(event Event) <-chan struct{}
	dispatch(ctx context.Context, event Event) <-chan struct{}
	stop()
	start(Active)
}

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

type Element = elements.NamedElement

type Model struct {
	state
	namespace map[string]elements.NamedElement
	elements  []RedifinableElement
}

func (model *Model) Namespace() map[string]elements.NamedElement {
	return model.namespace
}

func (model *Model) Push(partial RedifinableElement) {
	model.elements = append(model.elements, partial)
}

type RedifinableElement = func(model *Model, stack []elements.NamedElement) elements.NamedElement

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

type behavior[T Active] struct {
	element
	method func(ctx context.Context, hsm T, event Event)
}

/******* Constraint *******/

type constraint[T Active] struct {
	element
	expression func(ctx context.Context, hsm T, event Event) bool
}

/******* Events *******/
type Event = elements.Event

var noevent = Event{}

type queue struct {
	mutex     sync.RWMutex
	events    []Event
	partition uint64
}

func (q *queue) pop() (Event, bool) {
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

func (q *queue) push(event Event) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if kind.IsKind(event.Kind, kind.CompletionEvent) {
		q.events = append(q.events[q.partition:], append([]Event{event}, q.events[:q.partition]...)...)
		q.partition++
	} else {
		q.events = append(q.events, event)
	}
}

func apply(model *Model, stack []elements.NamedElement, partials ...RedifinableElement) {
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
		namespace: map[string]elements.NamedElement{},
		elements:  redifinableElements,
	}

	stack := []elements.NamedElement{&model}
	for len(model.elements) > 0 {
		elements := model.elements
		model.elements = []RedifinableElement{}
		apply(&model, stack, elements...)
	}
	return model
}

func find(stack []elements.NamedElement, maybeKinds ...uint64) elements.NamedElement {
	for i := len(stack) - 1; i >= 0; i-- {
		if kind.IsKind(stack[i].Kind(), maybeKinds...) {
			return stack[i]
		}
	}
	return nil
}

func get[T elements.NamedElement](model *Model, name string) T {
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
	return func(graph *Model, stack []elements.NamedElement) elements.NamedElement {
		owner := find(stack, kind.StateMachine, kind.State)
		if owner == nil {
			panic(fmt.Errorf("state \"%s\" must be called within Define() or State()", name))
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
	return func(model *Model, stack []elements.NamedElement) elements.NamedElement {
		owner := find(stack, kind.Vertex)
		if name == "" {
			name = fmt.Sprintf("transition_%d", len(model.namespace))
		}
		if owner == nil {
			panic(fmt.Errorf("transition \"%s\" must be called within a State() or Define()", name))
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
			model.Push(func(model *Model, stack []elements.NamedElement) elements.NamedElement {
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
	return func(model *Model, stack []elements.NamedElement) elements.NamedElement {
		owner := find(stack, kind.Transition)
		if owner == nil {
			panic(fmt.Errorf("hsm.Source() must be called within a hsm.Transition()"))
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
			model.Push(func(model *Model, stack []elements.NamedElement) elements.NamedElement {
				if _, ok := model.namespace[name]; !ok {
					panic(fmt.Errorf("missing source \"%s\" for transition \"%s\"", name, transition.QualifiedName()))
				}
				return owner
			})
		case RedifinableElement:
			element := any(nameOrPartialElement).(RedifinableElement)(model, stack)
			if element == nil {
				panic(fmt.Errorf("transition \"%s\" source is nil", transition.QualifiedName()))
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
	return func(model *Model, stack []elements.NamedElement) elements.NamedElement {
		owner := find(stack, kind.Transition)
		if owner == nil {
			panic(fmt.Errorf("Target() must be called within Transition()"))
		}
		transition := owner.(*transition)
		if transition.target != "" {
			panic(fmt.Errorf("transition \"%s\" already has target \"%s\"", transition.QualifiedName(), transition.target))
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
			model.Push(func(model *Model, stack []elements.NamedElement) elements.NamedElement {
				if _, exists := model.namespace[qualifiedName]; !exists {
					panic(fmt.Errorf("missing target \"%s\" for transition \"%s\"", target, transition.QualifiedName()))
				}
				return transition
			})
		case RedifinableElement:
			targetElement := target(model, stack)
			if targetElement == nil {
				panic(fmt.Errorf("transition \"%s\" target is nil", transition.QualifiedName()))
			}
			qualifiedName = targetElement.QualifiedName()
		}

		transition.target = qualifiedName
		return transition
	}
}

func Effect[T Active](fn func(ctx context.Context, hsm T, event Event), maybeName ...string) RedifinableElement {
	name := ".effect"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(model *Model, stack []elements.NamedElement) elements.NamedElement {
		owner := find(stack, kind.Transition)
		if owner == nil {
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

func Guard[T Active](fn func(ctx context.Context, hsm T, event Event) bool, maybeName ...string) RedifinableElement {
	name := ".guard"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(model *Model, stack []elements.NamedElement) elements.NamedElement {
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
	return func(model *Model, stack []elements.NamedElement) elements.NamedElement {
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
	return func(model *Model, stack []elements.NamedElement) elements.NamedElement {
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
		if defaultTransition := get[elements.Transition](model, element.transitions[len(element.transitions)-1]); defaultTransition != nil {
			if defaultTransition.Guard() != "" {
				panic(fmt.Errorf("the last transition of choice state \"%s\" cannot have a guard", qualifiedName))
			}
		}
		return element
	}
}

func Entry[T Active](fn func(ctx context.Context, hsm T, event Event), maybeName ...string) RedifinableElement {
	name := ".entry"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(model *Model, stack []elements.NamedElement) elements.NamedElement {
		owner := find(stack, kind.State)
		if owner == nil {
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

func Activity[T Active](fn func(ctx context.Context, hsm T, event Event), maybeName ...string) RedifinableElement {
	name := ".activity"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(model *Model, stack []elements.NamedElement) elements.NamedElement {
		owner := find(stack, kind.State)
		if owner == nil {
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

func Exit[T Active](fn func(ctx context.Context, hsm T, event Event), maybeName ...string) RedifinableElement {
	name := ".exit"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(model *Model, stack []elements.NamedElement) elements.NamedElement {
		owner := find(stack, kind.State)
		if owner == nil {
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
	return func(model *Model, stack []elements.NamedElement) elements.NamedElement {
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

func After[T Active](expr func(ctx context.Context, hsm T) time.Duration, maybeName ...string) RedifinableElement {
	name := ".after"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(builder *Model, stack []elements.NamedElement) elements.NamedElement {
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

func Final(name string) RedifinableElement {
	return func(builder *Model, stack []elements.NamedElement) elements.NamedElement {
		panic("not implemented")
	}
}

type HSM struct {
	Active
}

func (hsm *HSM) start(active Active) {
	if hsm == nil || hsm.Active != nil {
		return
	}
	hsm.Active = active
	active.start(hsm)
}

func (hsm HSM) Dispatch(event Event) <-chan struct{} {
	if hsm.Active == nil {
		return done(event.Done)
	}
	return hsm.Active.Dispatch(event)
}

func (hsm HSM) State() string {
	if hsm.Active == nil {
		return ""
	}
	return hsm.Active.State()
}

func (hsm HSM) stop() {
	if hsm.Active == nil {
		return
	}
	hsm.Active.stop()
}

type subcontext = context.Context

type hsm[T Active] struct {
	subcontext
	behavior[T]
	state      elements.NamedElement
	model      *Model
	active     map[string]*active
	queue      queue
	processing atomic.Bool
	context    T
	trace      Trace
	event      chan Event
}

type active struct {
	subcontext
	cancel  context.CancelFunc
	channel chan struct{}
}

type Trace func(ctx context.Context, step string, data ...any) (context.Context, func(...any))

type Config struct {
	Trace Trace
	Id    string
}

type key[T any] struct{}

var Keys = struct {
	All key[*sync.Map]
	HSM key[HSM]
}{
	All: key[*sync.Map]{},
	HSM: key[HSM]{},
}

func Start[T Active](ctx context.Context, sm T, model *Model, config ...Config) T {
	hsm := &hsm[T]{
		behavior: behavior[T]{
			element: element{
				kind:          kind.StateMachine,
				qualifiedName: model.QualifiedName(),
			},
		},
		model:   model,
		active:  map[string]*active{},
		context: sm,
		queue:   queue{},
		event:   make(chan Event),
	}
	if len(config) > 0 {
		hsm.trace = config[0].Trace
		hsm.id = config[0].Id
	}
	all, ok := ctx.Value(Keys.All).(*sync.Map)
	if !ok {
		all = &sync.Map{}
	}
	hsm.subcontext = context.WithValue(context.WithValue(context.Background(), Keys.All, all), Keys.HSM, hsm)
	all.Store(hsm, struct{}{})
	hsm.method = func(ctx context.Context, _ T, event Event) {
		hsm.processing.Store(true)
		defer hsm.processing.Store(false)
		hsm.state = hsm.initial(hsm.subcontext, &model.state, event)
	}
	sm.start(hsm)
	return sm
}

func (sm *hsm[T]) State() string {
	if sm == nil {
		return ""
	}
	if sm.state == nil {
		return ""
	}
	return sm.state.QualifiedName()
}

func (sm *hsm[T]) start(active Active) {
	sm.execute(sm.subcontext, &sm.behavior, noevent)
}

func (sm *hsm[T]) Dispatch(event Event) <-chan struct{} {
	return sm.dispatch(sm.subcontext, event)
}

func (sm *hsm[T]) stop() {
	if sm == nil {
		return
	}
	ctx := sm.subcontext
	if sm.trace != nil {
		var end func(...any)
		ctx, end = sm.trace(ctx, "Terminate", sm.state)
		defer end()
	}
	var ok bool
	for sm.state != nil {
		sm.exit(ctx, sm.state, noevent)
		sm.state, ok = sm.model.namespace[sm.state.Owner()]
		if !ok {
			break
		}
	}
	all, ok := sm.Value(Keys.All).(*sync.Map)
	if !ok {
		return
	}
	all.Delete(sm)
}

func Stop(ctx context.Context) {
	if hsm, ok := FromContext(ctx); ok {
		hsm.stop()
	}
}

func (sm *hsm[T]) activate(id string) *active {
	current, ok := sm.active[id]
	if !ok {
		current = &active{
			channel: make(chan struct{}, 1),
		}
		sm.active[id] = current
	}
	current.subcontext, current.cancel = context.WithCancel(sm)
	return current
}

func (sm *hsm[T]) enter(ctx context.Context, element elements.NamedElement, event Event, defaultEntry bool) elements.NamedElement {
	if sm == nil {
		return nil
	}
	if sm.trace != nil {
		var end func(...any)
		ctx, end = sm.trace(sm, "enter", element)
		defer end()
	}
	switch element.Kind() {
	case kind.State:
		state := element.(*state)
		if entry := get[*behavior[T]](sm.model, state.entry); entry != nil {
			sm.execute(ctx, entry, event)
		}
		activity := get[*behavior[T]](sm.model, state.activity)
		if activity != nil {
			sm.execute(ctx, activity, event)
		}
		for _, qualifiedName := range state.transitions {
			if element := get[*transition](sm.model, qualifiedName); element != nil {
				for _, event := range element.Events() {
					switch event.Kind {
					case kind.TimeEvent:
						ctx := sm.activate(event.Id)
						go func(ctx *active, event Event) {
							duration := event.Data.(func(ctx context.Context, hsm T) time.Duration)(
								ctx,
								sm.context,
							)
							timer := time.NewTimer(duration)
							defer timer.Stop()
							select {
							case <-ctx.Done():
								break
							case <-sm.Done():
								break
							case <-timer.C:
								timer.Stop()
								sm.dispatch(ctx, event)
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
		return sm.initial(ctx, element, event)
	case kind.Choice:
		for _, qualifiedName := range element.(*vertex).transitions {
			if transition := get[*transition](sm.model, qualifiedName); transition != nil {
				if constraint := get[*constraint[T]](sm.model, transition.Guard()); constraint != nil {
					if !sm.evaluate(ctx, constraint, event) {
						continue
					}
				}
				return sm.transition(ctx, element, transition, event)
			}
		}
	}
	return nil
}

func (sm *hsm[T]) initial(ctx context.Context, element elements.NamedElement, event Event) elements.NamedElement {
	if sm == nil || element == nil {
		return nil
	}
	if sm.trace != nil {
		var end func(...any)
		ctx, end = sm.trace(sm, "initial", element)
		defer end()
	}
	var qualifiedName string
	if element.QualifiedName() == "/" {
		qualifiedName = "/.initial"
	} else {
		qualifiedName = element.QualifiedName() + "/.initial"
	}
	if initial := get[*vertex](sm.model, qualifiedName); initial != nil {
		if len(initial.transitions) > 0 {
			if transition := get[*transition](sm.model, initial.transitions[0]); transition != nil {
				return sm.transition(ctx, element, transition, event)
			}
		}
	}
	return element
}

func (sm *hsm[T]) exit(ctx context.Context, element elements.NamedElement, event Event) {
	if sm == nil || element == nil {
		return
	}
	if sm.trace != nil {
		var end func(...any)
		ctx, end = sm.trace(sm, "exit", element)
		defer end()
	}
	if state, ok := element.(*state); ok {
		for _, qualifiedName := range state.transitions {
			if element := get[*transition](sm.model, qualifiedName); element != nil {
				for _, event := range element.Events() {
					switch event.Kind {
					case kind.TimeEvent:
						active, ok := sm.active[event.Id]
						if ok {
							active.cancel()
						}
					}
				}
			}
		}
		if activity := get[*behavior[T]](sm.model, state.activity); activity != nil {
			sm.terminate(ctx, activity)
		}
		if exit := get[*behavior[T]](sm.model, state.exit); exit != nil {
			sm.execute(ctx, exit, event)
		}
	}

}

func (sm *hsm[T]) execute(ctx context.Context, element *behavior[T], event Event) {
	if sm == nil || element == nil {
		return
	}
	var end func(...any)
	if sm.trace != nil {
		var end func(...any)
		ctx, end = sm.trace(ctx, "execute", element, event)
		defer end()
	}
	switch element.Kind() {
	case kind.Concurrent:
		ctx := sm.activate(element.QualifiedName())
		go func(ctx *active, end func(...any)) {
			if end != nil {
				defer end()
			}
			element.method(ctx, sm.context, event)
			ctx.channel <- struct{}{}
		}(ctx, end)
	default:
		element.method(ctx, sm.context, event)
	}

}

func (sm *hsm[T]) evaluate(ctx context.Context, guard *constraint[T], event Event) bool {
	if sm == nil || guard == nil || guard.expression == nil {
		return true
	}
	if sm.trace != nil {
		var end func(...any)
		ctx, end = sm.trace(ctx, "evaluate", guard, event)
		defer end()
	}
	return guard.expression(
		ctx,
		sm.context,
		event,
	)
}

func (sm *hsm[T]) transition(ctx context.Context, current elements.NamedElement, transition *transition, event Event) elements.NamedElement {
	if sm == nil {
		return nil
	}
	if sm.trace != nil {
		var end func(...any)
		ctx, end = sm.trace(ctx, "transition", current, transition, event)
		defer end()
	}
	path, ok := transition.paths[current.QualifiedName()]
	if !ok {
		return nil
	}
	for _, exiting := range path.exit {
		current, ok = sm.model.namespace[exiting]
		if !ok {
			return nil
		}
		sm.exit(ctx, current, event)
	}
	if effect := get[*behavior[T]](sm.model, transition.effect); effect != nil {
		sm.execute(ctx, effect, event)
	}
	if kind.IsKind(transition.kind, kind.Internal) {
		return current
	}
	for _, entering := range path.enter {
		next, ok := sm.model.namespace[entering]
		if !ok {
			return nil
		}
		defaultEntry := entering == transition.target
		current = sm.enter(ctx, next, event, defaultEntry)
		if defaultEntry {
			return current
		}
	}
	current, ok = sm.model.namespace[transition.target]
	if !ok {
		return nil
	}
	return current
}

func (sm *hsm[T]) terminate(ctx context.Context, behavior elements.NamedElement) {
	if sm == nil || behavior == nil {
		return
	}
	if sm.trace != nil {
		_, end := sm.trace(ctx, "terminate", behavior)
		defer end()
	}
	active, ok := sm.active[behavior.QualifiedName()]
	if !ok {
		return
	}
	active.cancel()
	<-active.channel

}

func (sm *hsm[T]) enabled(ctx context.Context, source elements.Vertex, event Event) *transition {
	if sm == nil {
		return nil
	}
	for _, transitionQualifiedName := range source.Transitions() {
		transition := get[*transition](sm.model, transitionQualifiedName)
		if transition == nil {
			continue
		}
		for _, evt := range transition.Events() {
			if matched, err := path.Match(evt.Name, event.Name); err != nil || !matched {
				continue
			}
			if guard := get[*constraint[T]](sm.model, transition.Guard()); guard != nil {
				if !sm.evaluate(ctx, guard, event) {
					continue
				}
			}
			return transition
		}
	}
	return nil
}

func (sm *hsm[T]) process(ctx context.Context, event Event) {
	if sm.processing.Load() {
		return
	}
	sm.processing.Store(true)
	defer sm.processing.Store(false)
	ok := true
	// queued := false
	for ok {
		state := sm.state.QualifiedName()
		for state != "/" {
			source := get[elements.Vertex](sm.model, state)
			if source == nil {
				break
			}
			if transition := sm.enabled(ctx, source, event); transition != nil {
				sm.state = sm.transition(ctx, sm.state, transition, event)
				break
			}
			state = source.Owner()
		}
		done(event.Done)
		event, ok = sm.queue.pop()
	}
}

func (sm *hsm[T]) dispatch(ctx context.Context, event Event) <-chan struct{} {
	if sm == nil {
		return done(event.Done)
	}
	if sm.state == nil {
		return done(event.Done)
	}
	if event.Kind == 0 {
		event.Kind = kind.Event
	}
	if sm.trace != nil {
		var end func(...any)
		_, end = sm.trace(ctx, "Dispatch", event)
		defer end()
	}
	if sm.processing.Load() {
		sm.queue.push(event)
		return event.Done
	}
	sm.process(ctx, event)
	return event.Done
}

func Dispatch(ctx context.Context, event Event) <-chan struct{} {
	if hsm, ok := FromContext(ctx); ok {
		return hsm.dispatch(ctx, event)
	}
	return done(event.Done)
}

func DispatchAll(ctx context.Context, event Event) <-chan struct{} {
	all, ok := ctx.Value(Keys.All).(*sync.Map)
	if !ok {
		return done(event.Done)
	}
	all.Range(func(value any, _ any) bool {
		maybeSM, ok := value.(Active)
		if !ok {
			return true
		}
		maybeSM.dispatch(ctx, event)
		return true
	})
	return event.Done
}

func FromContext(ctx context.Context) (Active, bool) {
	hsm, ok := ctx.Value(Keys.HSM).(Active)
	if ok {
		return hsm, true
	}
	return nil, false
}

func done(channel chan struct{}) <-chan struct{} {
	if channel == nil {
		return nil
	}
	select {
	case _, ok := <-channel:
		if !ok {
			return channel
		}
		break
	default:
		break
	}
	close(channel)
	return channel
}
