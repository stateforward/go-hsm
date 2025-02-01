package hsm

import (
	"context"
	"encoding/json"
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
	"github.com/stateforward/go-hsm/queue"
)

var Kinds = kind.Kinds()

/******* Element *******/

type element struct {
	kind          uint64
	qualifiedName string
	id            string
	metadata      map[string]any
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

func (element *element) Metadata() map[string]any {
	if element == nil {
		return nil
	}
	return element.metadata
}

/******* Model *******/

type Element = embedded.Element

type Model struct {
	state
	namespace map[string]embedded.Element
	elements  []RedifinableElement
}

func (model *Model) Namespace() map[string]embedded.Element {
	return model.namespace
}

func (model *Model) Push(partial RedifinableElement) {
	model.elements = append(model.elements, partial)
}

type RedifinableElement = func(model *Model, stack []embedded.Element) embedded.Element

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
	events []embedded.Event
	paths  map[string]paths
}

func (transition *transition) Guard() string {
	return transition.guard
}

func (transition *transition) Effect() string {
	return transition.effect
}

func (transition *transition) Events() []embedded.Event {
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
	method func(hsm Active[T], event Event)
}

/******* Constraint *******/

type constraint[T context.Context] struct {
	element
	expression func(hsm Active[T], event Event) bool
}

/******* Events *******/

type Event = embedded.Event

type event struct {
	element
	data any
}

func (event *event) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"kind": event.kind,
		"name": event.qualifiedName,
		"id":   event.id,
		"data": event.data,
	})
}

func (event *event) UnmarshalJSON(data []byte) error {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if kind, ok := m["kind"].(uint64); ok {
		event.kind = kind
	}
	if name, ok := m["name"].(string); ok {
		event.qualifiedName = name
	}
	if id, ok := m["id"].(string); ok {
		event.id = id
	}
	event.data = m["data"]
	return nil
}

func (event *event) Name() string {
	if event == nil {
		return ""
	}
	return event.qualifiedName
}

func (event *event) Data() any {
	if event == nil {
		return nil
	}
	return event.data
}

func apply(model *Model, stack []embedded.Element, partials ...RedifinableElement) {
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
		namespace: map[string]embedded.Element{},
		elements:  redifinableElements,
	}

	stack := []embedded.Element{&model}
	for len(model.elements) > 0 {
		elements := model.elements
		model.elements = []RedifinableElement{}
		apply(&model, stack, elements...)
	}
	return model
}

func find(stack []embedded.Element, maybeKinds ...uint64) embedded.Element {
	for i := len(stack) - 1; i >= 0; i-- {
		if kind.IsKind(stack[i].Kind(), maybeKinds...) {
			return stack[i]
		}
	}
	return nil
}

func get[T embedded.Element](model *Model, name string) T {
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
	return func(graph *Model, stack []embedded.Element) embedded.Element {
		owner := find(stack, kind.StateMachine, kind.State)
		if owner == nil {
			slog.Error("state must be called within a StateMachine or State")
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
	return func(model *Model, stack []embedded.Element) embedded.Element {
		owner := find(stack, kind.Vertex)
		if owner == nil {
			panic(fmt.Errorf("transition must be called within a State or StateMachine"))
		}
		if name == "" {
			name = fmt.Sprintf("transition_%d", len(model.namespace))
		}
		transition := &transition{
			events: []embedded.Event{},
			element: element{
				kind:          kind.Transition,
				qualifiedName: path.Join(owner.QualifiedName(), name),
			},
			paths: map[string]paths{},
		}
		model.namespace[transition.QualifiedName()] = transition
		stack = append(stack, transition)
		apply(model, stack, partialElements...)
		if transition.source == "" {
			transition.source = owner.QualifiedName()
		}
		sourceElement, ok := model.namespace[transition.source]
		if !ok {
			panic(fmt.Errorf("missing source %s", transition.source))
		}
		switch source := sourceElement.(type) {
		case *state:
			source.transitions = append(source.transitions, transition.QualifiedName())
		case *vertex:
			source.transitions = append(source.transitions, transition.QualifiedName())
		}
		if len(transition.events) == 0 && !kind.IsKind(sourceElement.Kind(), kind.Pseudostate) {

			// TODO: completion transition
			qualifiedName := path.Join(transition.source, ".completion")
			transition.events = append(transition.events, &event{
				element: element{kind: kind.CompletionEvent, qualifiedName: qualifiedName},
			})
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
			model.Push(func(model *Model, stack []embedded.Element) embedded.Element {
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
		transition.metadata = map[string]any{
			"source": transition.source,
			"target": transition.target,
			"guard":  transition.guard,
			"effect": transition.effect,
		}
		return transition
	}
}

func Source[T interface{ RedifinableElement | string }](nameOrPartialElement T) RedifinableElement {
	return func(model *Model, stack []embedded.Element) embedded.Element {
		owner := find(stack, kind.Transition)
		if owner == nil {
			panic(fmt.Errorf("source must be called within a Transition"))
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
			model.Push(func(model *Model, stack []embedded.Element) embedded.Element {
				if _, ok := model.namespace[name]; !ok {
					slog.Error("missing source", "id", name)
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
		owner.(*transition).source = name
		return owner
	}
}

func Defer(events ...uint64) RedifinableElement {
	panic("not implemented")
}

func Target[T interface{ RedifinableElement | string }](nameOrPartialElement T) RedifinableElement {
	return func(model *Model, stack []embedded.Element) embedded.Element {
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
			model.Push(func(model *Model, stack []embedded.Element) embedded.Element {
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

func Effect[T context.Context](fn func(hsm Active[T], event Event), maybeName ...string) RedifinableElement {
	name := ".effect"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(model *Model, stack []embedded.Element) embedded.Element {
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

func Guard[T context.Context](fn func(hsm Active[T], event Event) bool, maybeName ...string) RedifinableElement {
	name := ".guard"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(model *Model, stack []embedded.Element) embedded.Element {
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
	return func(model *Model, stack []embedded.Element) embedded.Element {
		owner := find(stack, kind.State)
		if owner == nil {
			panic(fmt.Errorf("initial must be called within a State"))
		}
		initial := &vertex{
			element: element{kind: kind.Initial, qualifiedName: path.Join(owner.QualifiedName(), name)},
		}
		if model.namespace[initial.QualifiedName()] != nil {
			panic(fmt.Errorf("initial %s state already exists for %s", initial.QualifiedName(), owner.QualifiedName()))
		}
		model.namespace[initial.QualifiedName()] = initial
		stack = append(stack, initial)
		transition := (Transition(Source(initial.QualifiedName()), partialElements...)(model, stack)).(*transition)
		// validation logic
		if transition.guard != "" {
			panic(fmt.Errorf("initial %s cannot have a guard", initial.QualifiedName()))
		}
		if len(transition.events) > 0 {
			panic(fmt.Errorf("initial %s cannot have triggers", initial.QualifiedName()))
		}
		if !strings.HasPrefix(transition.target, owner.QualifiedName()) {
			panic(fmt.Errorf("initial %s must target a nested state not %s", initial.QualifiedName(), transition.target))
		}
		if len(initial.transitions) > 1 {
			panic(fmt.Errorf("initial %s cannot have multiple transitions %v", initial.QualifiedName(), initial.transitions))
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
	return func(model *Model, stack []embedded.Element) embedded.Element {
		owner := find(stack, kind.State, kind.Transition)
		if owner == nil {
			panic(fmt.Errorf("choice must be called within a State or Transition"))
		} else if kind.IsKind(owner.Kind(), kind.Transition) {
			transition := owner.(*transition)
			source := transition.source
			owner = model.namespace[source]
			if owner == nil {
				panic(fmt.Errorf("you must specifiy a source when defining a choice within transition %s, with target %s", transition.QualifiedName(), transition.target))
			} else if kind.IsKind(owner.Kind(), kind.Pseudostate) {
				// pseudostates aren't a namespace, so we need to find the containing state
				owner = find(stack, kind.State)
				if owner == nil {
					panic(fmt.Errorf("choice must be called within a State"))
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
			slog.Error("choice must have at least one transition")
			panic(fmt.Errorf("choice must have at least one transition"))
		}
		if defaultTransition := get[embedded.Transition](model, element.transitions[len(element.transitions)-1]); defaultTransition != nil {
			if defaultTransition.Guard() != "" {
				panic(fmt.Errorf("the last transition of a choice state cannot have a guard"))
			}
		}
		return element
	}
}

func Entry[T context.Context](fn func(ctx Active[T], event Event), maybeName ...string) RedifinableElement {
	name := ".entry"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(model *Model, stack []embedded.Element) embedded.Element {
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

func Activity[T context.Context](fn func(hsm Active[T], event Event), maybeName ...string) RedifinableElement {
	name := ".activity"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(model *Model, stack []embedded.Element) embedded.Element {
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

func Exit[T context.Context](fn func(hsm Active[T], event Event), maybeName ...string) RedifinableElement {
	name := ".exit"
	if len(maybeName) > 0 {
		name = maybeName[0]
	}
	return func(model *Model, stack []embedded.Element) embedded.Element {
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

func Trigger[T interface{ string | *event }](events ...T) RedifinableElement {
	return func(model *Model, stack []embedded.Element) embedded.Element {
		owner := find(stack, kind.Transition)
		if owner == nil {
			panic(fmt.Errorf("trigger must be called within a Transition"))
		}
		transition := owner.(*transition)
		for _, eventOrName := range events {
			switch any(eventOrName).(type) {
			case string:
				name := any(eventOrName).(string)
				transition.events = append(transition.events, &event{
					element: element{kind: kind.Event, qualifiedName: name},
				})
			case *event:
				event := any(eventOrName).(*event)
				transition.events = append(transition.events, event)
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
	return func(builder *Model, stack []embedded.Element) embedded.Element {
		owner := find(stack, kind.Transition)
		if owner == nil {
			panic(fmt.Errorf("after must be called within a Transition"))
		}
		qualifiedName := path.Join(owner.QualifiedName(), strconv.Itoa(len(owner.(*transition).events)), name)
		owner.(*transition).events = append(owner.(*transition).events, &event{
			element: element{kind: kind.TimeEvent, qualifiedName: qualifiedName},
			data:    expr,
		})
		return owner
	}
}

var pool = sync.Pool{
	New: func() any {
		return &event{}
	},
}

func NewEvent(name string, maybeData ...any) *event {
	var data any
	if len(maybeData) > 0 {
		data = maybeData[0]
	}
	event := pool.Get().(*event)
	// id := ""
	// uid, err := uuid.NewV7()
	// if err != nil {
	// 	id = uid.String()
	// }
	event.element = element{kind: kind.Event, qualifiedName: name}
	event.data = data
	return event
}

func Final(name string) RedifinableElement {
	return func(builder *Model, stack []embedded.Element) embedded.Element {
		panic("not implemented")
	}
}

type subcontext = context.Context

type HSM[T context.Context] struct {
	subcontext
	behavior[T]
	state      embedded.Element
	model      *Model
	active     map[string]*Active[T]
	queue      *queue.Queue
	processing atomic.Bool
	Context    T
	trace      Trace
}

// type HSM[T context.Context] struct {
// 	subcontext
// 	*statemachine[T]
// }

type Active[T context.Context] struct {
	subcontext
	*HSM[T]
	cancel  context.CancelFunc
	channel chan struct{}
}

func (active *Active[T]) Dispatch(event Event) {
	if active.cancel != nil {
		go active.HSM.Dispatch(event)
	} else {
		active.HSM.Dispatch(event)
	}
}

type Trace func(ctx context.Context, step string, elements ...embedded.Element) func(...any)

func WithTrace[T context.Context](hsm *HSM[T], trace Trace) *HSM[T] {
	hsm.trace = trace
	return hsm
}

type key[T any] struct{}

var Keys = struct {
	All key[*sync.Map]
	HSM key[*HSM[context.Context]]
}{
	All: key[*sync.Map]{},
	HSM: key[*HSM[context.Context]]{},
}

func New[T context.Context](ctx T, model *Model) *HSM[T] {
	hsm := &HSM[T]{
		behavior: behavior[T]{
			element: element{
				kind:          kind.StateMachine,
				qualifiedName: model.QualifiedName(),
			},
		},
		model:   model,
		active:  map[string]*Active[T]{},
		Context: ctx,
		queue:   queue.New(),
	}
	all, ok := ctx.Value(Keys.All).(*sync.Map)
	if !ok {
		all = &sync.Map{}
	}
	all.Store(hsm, struct{}{})
	hsm.subcontext = context.WithValue(ctx, Keys.All, all)
	hsm.method = func(_ Active[T], event Event) {
		hsm.processing.Store(true)
		defer hsm.processing.Store(false)
		hsm.state = hsm.initial(&model.state, event)
	}
	hsm.execute(&hsm.behavior, nil)
	return hsm
}

func (sm *HSM[T]) State() string {
	if sm == nil {
		return ""
	}
	if sm.state == nil {
		return ""
	}
	return sm.state.QualifiedName()
}

func (sm *HSM[T]) Terminate() {
	if sm == nil {
		return
	}
	if sm.trace != nil {
		defer sm.trace(sm.Context, "Terminate", sm.state)()
	}
	var ok bool
	for sm.state != nil {
		sm.exit(sm.state, nil)
		sm.state, ok = sm.model.namespace[sm.state.Owner()]
		if !ok {
			break
		}
	}
	// all, ok := sm.Value(Keys.All).(*sync.Map)
	// if !ok {
	// 	return
	// }
	// all.Delete(sm)
}

func (sm *HSM[T]) activate(element embedded.Element) *Active[T] {
	current, ok := sm.active[element.QualifiedName()]
	if !ok {
		current = &Active[T]{
			channel: make(chan struct{}, 1),
		}
		sm.active[element.QualifiedName()] = current
	}
	current.subcontext, current.cancel = context.WithCancel(sm.Context)
	return current
}

func (sm *HSM[T]) enter(element embedded.Element, event Event, defaultEntry bool) embedded.Element {
	if sm == nil {
		return nil
	}
	if sm.trace != nil {
		defer sm.trace(sm.Context, "enter", element)()
	}
	switch element.Kind() {
	case kind.State:
		state := element.(*state)
		if entry := get[*behavior[T]](sm.model, state.entry); entry != nil {
			sm.execute(entry, event)
		}
		activity := get[*behavior[T]](sm.model, state.activity)
		if activity != nil {
			sm.execute(activity, event)
		}
		for _, qualifiedName := range state.transitions {
			if element := get[*transition](sm.model, qualifiedName); element != nil {
				for _, event := range element.Events() {
					switch event.Kind() {
					case kind.TimeEvent:
						ctx := sm.activate(event)
						go func(ctx *Active[T], event embedded.Event) {
							duration := event.Data().(func(hsm Active[T]) time.Duration)(
								Active[T]{
									subcontext: ctx,
									HSM:        sm,
								},
							)
							timer := time.NewTimer(duration)
							defer timer.Stop()
							select {
							case <-ctx.Done():
								break
							case <-sm.Context.Done():
								break
							case <-timer.C:
								timer.Stop()
								sm.Dispatch(event)
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
		return sm.initial(element, event)
	case kind.Choice:
		for _, qualifiedName := range element.(*vertex).transitions {
			if transition := get[*transition](sm.model, qualifiedName); transition != nil {
				if constraint := get[*constraint[T]](sm.model, transition.Guard()); constraint != nil {
					if !sm.evaluate(constraint, event) {
						continue
					}
				}
				return sm.transition(element, transition, event)
			}
		}
	}
	return nil
}

func (sm *HSM[T]) initial(element embedded.Element, event Event) embedded.Element {
	if sm == nil || element == nil {
		return nil
	}
	if sm.trace != nil {
		defer sm.trace(sm.Context, "initial", element)()
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
				return sm.transition(element, transition, event)
			}
		}
	}
	return element
}

func (sm *HSM[T]) exit(element embedded.Element, event Event) {
	if sm == nil || element == nil {
		return
	}
	if sm.trace != nil {
		defer sm.trace(sm.Context, "exit", element)()
	}
	if state, ok := element.(*state); ok {
		for _, qualifiedName := range state.transitions {
			if element := get[*transition](sm.model, qualifiedName); element != nil {
				for _, event := range element.Events() {
					switch event.Kind() {
					case kind.TimeEvent:
						active, ok := sm.active[event.QualifiedName()]
						if ok {
							active.cancel()
						}
					}
				}
			}
		}
		if activity := get[*behavior[T]](sm.model, state.activity); activity != nil {
			sm.terminate(activity)
		}
		if exit := get[*behavior[T]](sm.model, state.exit); exit != nil {
			sm.execute(exit, event)
		}
	}

}

func (sm *HSM[T]) execute(element *behavior[T], event Event) {
	if sm == nil || element == nil {
		return
	}
	var end func(...any)
	if sm.trace != nil {
		end = sm.trace(sm.Context, "execute", element)
	}
	switch element.Kind() {
	case kind.Concurrent:
		ctx := sm.activate(element)
		go func(ctx *Active[T], end func(...any)) {
			if end != nil {
				defer end()
			}
			element.method(Active[T]{
				subcontext: ctx,
				HSM:        sm,
			}, event)
			ctx.channel <- struct{}{}
		}(ctx, end)
	default:
		if end != nil {
			defer end()
		}
		element.method(Active[T]{
			subcontext: sm.Context,
			HSM:        sm,
		}, event)

	}

}

func (sm *HSM[T]) evaluate(guard *constraint[T], event Event) bool {
	if sm == nil || guard == nil || guard.expression == nil {
		return true
	}
	if sm.trace != nil {
		defer sm.trace(sm.Context, "evaluate", guard)()
	}
	return guard.expression(
		Active[T]{
			subcontext: sm.Context,
			HSM:        sm,
		},
		event,
	)
}

func (sm *HSM[T]) transition(current embedded.Element, transition *transition, event Event) embedded.Element {
	if sm == nil {
		return nil
	}
	if sm.trace != nil {
		defer sm.trace(sm.Context, "transition", transition)()
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
		sm.exit(current, event)
	}
	if effect := get[*behavior[T]](sm.model, transition.effect); effect != nil {
		sm.execute(effect, event)
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
		current = sm.enter(next, event, defaultEntry)
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

func (sm *HSM[T]) terminate(behavior *behavior[T]) {
	if sm == nil || behavior == nil {
		return
	}
	if sm.trace != nil {
		defer sm.trace(sm.Context, "terminate", behavior)()
	}
	active, ok := sm.active[behavior.QualifiedName()]
	if !ok {
		return
	}
	active.cancel()
	<-active.channel

}

func (sm *HSM[T]) enabled(source embedded.Vertex, event Event) *transition {
	if sm == nil {
		return nil
	}
	for _, transitionQualifiedName := range source.Transitions() {
		transition := get[*transition](sm.model, transitionQualifiedName)
		if transition == nil {
			continue
		}
		for _, evt := range transition.Events() {
			if matched, err := path.Match(evt.Name(), event.Name()); err != nil || !matched {
				continue
			}
			if guard := get[*constraint[T]](sm.model, transition.Guard()); guard != nil {
				if !sm.evaluate(guard, event) {
					continue
				}
			}
			return transition
		}
	}
	return nil
}

func (sm *HSM[T]) process(event embedded.Event) {
	if sm.processing.Load() {
		return
	}
	sm.processing.Store(true)
	defer sm.processing.Store(false)
	for event != nil {
		state := sm.state.QualifiedName()
		for state != "/" {
			source := get[embedded.Vertex](sm.model, state)
			if source == nil {
				break
			}
			if transition := sm.enabled(source, event); transition != nil {
				sm.state = sm.transition(sm.state, transition, event)
				break
			}
			state = source.Owner()
		}
		pool.Put(event)
		event = sm.queue.Pop()
	}
}

func (sm *HSM[T]) Dispatch(event Event) {
	if sm == nil {
		return
	}
	if sm.trace != nil {
		defer sm.trace(sm.Context, "Dispatch", event)()
	}
	if sm.state == nil {
		return
	}
	if sm.processing.Load() {
		sm.queue.Push(event)
		return
	}
	sm.process(event)
}

func (sm *HSM[T]) DispatchAll(event Event) {
	active, ok := sm.Value(Keys.All).(*sync.Map)
	if !ok {
		return
	}
	var end func(...any)
	if sm.trace != nil {
		end = sm.trace(sm, "DispatchAll", event)
	}
	go func(active *sync.Map, end func(...any)) {
		if end != nil {
			defer end()
		}
		active.Range(func(value any, _ any) bool {
			sm, ok := value.(embedded.Context)
			if !ok {
				return true
			}
			sm.Dispatch(event)
			return true
		})
	}(active, end)

}

type hsm interface {
	dispatch(event Event)
}

func FromContext[T context.Context](ctx T) (hsm, bool) {
	hsm, ok := ctx.Value(Keys.HSM).(hsm)
	if !ok {
		return nil, false
	}
	return hsm, true
}

// func DispatchAll(ctx context.Context, event Event) {
// 	active, ok := ctx.Value(Keys.All).(*sync.Map)
// 	if !ok {
// 		return
// 	}
// 	active.Range(func(value any, _ any) bool {
// 		sm, ok := value.(hsm)
// 		if !ok {
// 			return true
// 		}
// 		sm.dispatch(event)
// 		return true
// 	})
// }
