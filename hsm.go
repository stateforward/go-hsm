package hsm

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"sync"
	"time"
	"unique"

	"github.com/stateforward/go-hsm/elements"
	"github.com/stateforward/go-hsm/kinds"
)

type element struct {
	kind          uint64
	qualifiedName string
	// id            uint64
}

func (element *element) Kind() uint64 {
	return element.kind
}

func (element *element) Owner() string {
	return path.Dir(element.qualifiedName)
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

// func (element *element) Id() uint64 {
// 	if element == nil {
// 		return 0
// 	} else if element.id == 0 {
// 		id := fnv.New64()
// 		id.Write([]byte(element.qualifiedName))
// 		element.id = id.Sum64()
// 	}
// 	return element.id
// }

type model struct {
	element
	elements map[string]elements.Element
}

type Builder struct {
	model
	steps []Partial
}

func (model *model) Elements() map[string]elements.Element {
	return model.elements
}

type Partial = func(model *Builder, stack []elements.Element) elements.Element

type vertex struct {
	element
	transitions []string
}

func (vertex *vertex) Transitions() []string {
	return vertex.transitions
}

type state struct {
	vertex
	entry    string
	exit     string
	activity string
}

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
	events map[string]elements.Event
	paths  map[string]paths
}

func (transition *transition) Guard() string {
	return transition.guard
}

func (transition *transition) Effect() string {
	return transition.effect
}

func (transition *transition) Events() map[string]elements.Event {
	return transition.events
}

func (transition *transition) Source() string {
	return transition.source
}

func (transition *transition) Target() string {
	return transition.target
}

type behavior struct {
	element
	action any
}

func (behavior *behavior) Action() any {
	return behavior.action
}

type constraint[T context.Context] struct {
	element
	expression func(hsm HSM[T], event AnyEvent) bool
}

type active struct {
	channel chan struct{}
	cancel  context.CancelFunc
}

type AnyEvent = elements.Event

type event struct {
	element
	data any
}

func (event *event) Name() string {
	return event.qualifiedName
}

func (event *event) Data() any {
	return event.data
}

func apply(model *Builder, stack []elements.Element, partials ...Partial) {
	for _, partial := range partials {
		partial(model, stack)
	}
}

func Model(partials ...Partial) model {
	builder := Builder{
		model: model{
			element:  element{kind: kinds.StateMachine, qualifiedName: "/"},
			elements: map[string]elements.Element{},
		},
		steps: partials,
	}
	stack := []elements.Element{&builder}
	for len(builder.steps) > 0 {
		partials = builder.steps
		builder.steps = []Partial{}
		apply(&builder, stack, partials...)
	}
	return builder.model
}

func find(stack []elements.Element, maybeKinds ...uint64) elements.Element {
	for i := len(stack) - 1; i >= 0; i-- {
		if kinds.IsKind(stack[i].Kind(), maybeKinds...) {
			return stack[i]
		}
	}
	return nil
}

func get[T elements.Element](model *model, name string) T {
	var zero T
	if name == "" {
		return zero
	}
	if element, ok := model.elements[name]; ok {
		typed, ok := element.(T)
		if ok {
			return typed
		}
	}
	return zero
}

func State(name string, partialElements ...Partial) Partial {
	return func(model *Builder, stack []elements.Element) elements.Element {
		owner := find(stack, kinds.StateMachine, kinds.State)
		if owner == nil {
			slog.Error("state must be called within a StateMachine or State")
			panic(fmt.Errorf("state must be called within a StateMachine or State"))
		}
		element := &state{
			vertex: vertex{element: element{kind: kinds.State, qualifiedName: path.Join(owner.QualifiedName(), name)}, transitions: []string{}},
		}
		model.elements[element.QualifiedName()] = element
		stack = append(stack, element)
		apply(model, stack, partialElements...)
		return element
	}
}

// lca finds the Lowest Common Ancestor between two qualified state names in a hierarchical state machine.
// It takes two qualified names 'a' and 'b' as strings and returns their closest common ancestor.
//
// For example:
// - lca("/s/s1", "/s/s2") returns "/s"
// - lca("/s/s1", "/s/s1/s11") returns "/s/s1"
// - lca("/s/s1", "/s/s1") returns "/s/s1"
func lca(a, b string) string {
	if a == b {
		return a
	}
	if strings.HasPrefix(a, b) {
		return b
	}
	if strings.HasPrefix(b, a) {
		return a
	}
	return lca(path.Dir(a), path.Dir(b))
}

func Transition(partialElements ...Partial) Partial {
	return func(builder *Builder, stack []elements.Element) elements.Element {
		owner := find(stack, kinds.Vertex, kinds.StateMachine)
		if owner == nil {
			slog.Error("transition must be called within a State or StateMachine")
			panic(fmt.Errorf("transition must be called within a State or StateMachine"))
		}
		transition := &transition{
			events: map[string]elements.Event{},
			element: element{
				kind:          kinds.Transition,
				qualifiedName: path.Join(owner.QualifiedName(), fmt.Sprintf("transition_%d", len(builder.elements))),
			},
			paths: map[string]paths{},
		}
		builder.elements[transition.QualifiedName()] = transition
		stack = append(stack, transition)
		for _, partialElement := range partialElements {
			partialElement(builder, stack)
		}
		if transition.source == "" {
			transition.source = owner.QualifiedName()
		}
		sourceElement, ok := builder.elements[transition.source]
		if !ok {
			slog.Error("missing source", "id", transition.source)
			panic(fmt.Errorf("missing source %s", transition.source))
		}
		switch source := sourceElement.(type) {
		case *state:
			source.transitions = append(source.transitions, transition.QualifiedName())
		case *vertex:
			source.transitions = append(source.transitions, transition.QualifiedName())
		}
		if len(transition.events) == 0 && !kinds.IsKind(sourceElement.Kind(), kinds.Initial, kinds.Choice) {

			// completion transition
			slog.Error("completion transition not implemented", "source", transition.source, "target", transition.target)
			panic(fmt.Errorf("completion transition not implemented"))
		}
		var kind uint64
		if transition.target == transition.source {
			kind = kinds.Self
		} else if transition.target == "" {
			kind = kinds.Internal
		} else if match, err := path.Match(string(transition.source)+"/*", string(transition.target)); err == nil && match {
			kind = kinds.Local
		} else {
			kind = kinds.External
		}
		transition.kind = kind
		enter := []string{}
		entering := transition.target
		for !strings.HasPrefix(transition.source, entering) && entering != "/" && entering != "" {
			enter = append([]string{entering}, enter...)
			entering = path.Dir(entering)
		}
		if kinds.IsKind(transition.kind, kinds.Self) {
			enter = append(enter, sourceElement.QualifiedName())
		}
		if kinds.IsKind(sourceElement.Kind(), kinds.Initial) {
			transition.paths[path.Dir(sourceElement.QualifiedName())] = paths{
				enter: enter,
				exit:  []string{sourceElement.QualifiedName()},
			}
		} else {
			builder.steps = append(builder.steps, func(builder *Builder, stack []elements.Element) elements.Element {
				// precompute transition paths for the source state and nested states
				for qualifiedName, element := range builder.elements {
					if strings.HasPrefix(qualifiedName, transition.source) && kinds.IsKind(element.Kind(), kinds.State, kinds.StateMachine) {
						exit := []string{}
						if kind != kinds.Internal {
							exiting := element.QualifiedName()
							lca := lca(transition.source, transition.target)
							for exiting != lca {
								exit = append(exit, exiting)
								exiting = path.Dir(exiting)
							}
							if kinds.IsKind(transition.kind, kinds.Self) {
								exit = append(exit, sourceElement.QualifiedName())
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

func Source(id string) Partial {
	return func(model *Builder, stack []elements.Element) elements.Element {
		owner := find(stack, kinds.Transition)
		if owner == nil {
			slog.Error("source must be called within a Transition")
			panic(fmt.Errorf("source must be called within a Transition"))
		}

		if !path.IsAbs(id) {
			if ancestor := find(stack, kinds.State, kinds.StateMachine); ancestor != nil {
				id = path.Join(ancestor.QualifiedName(), id)
			}
		}
		if _, ok := model.elements[id]; !ok {
			slog.Error("missing source", "id", id)
			panic(fmt.Errorf("missing source %s", id))
		}
		owner.(*transition).source = id
		return owner
	}
}

var From = Source

func Defer(events ...uint64) Partial {
	panic("not implemented")
}

func Target[T interface{ Partial | string }](nameOrPartialElement T) Partial {
	return func(model *Builder, stack []elements.Element) elements.Element {
		owner := find(stack, kinds.Transition)
		if owner == nil {
			slog.Error("Target must be called within a Transition")
			panic(fmt.Errorf("Target must be called within a Transition"))
		}
		var qualifiedName string
		switch target := any(nameOrPartialElement).(type) {
		case string:
			qualifiedName = target
			if !path.IsAbs(qualifiedName) {
				if ancestor := find(stack, kinds.State, kinds.StateMachine); ancestor != nil {
					qualifiedName = path.Join(ancestor.QualifiedName(), qualifiedName)
				}
			}
			if _, exists := model.elements[qualifiedName]; !exists {
				slog.Warn("missing target", "target", qualifiedName, "source", owner.(*transition).source)
				panic(fmt.Errorf("missing target %s", target))
			}
		case Partial:
			targetElement := target(model, stack)
			if targetElement == nil {
				slog.Warn("target is nil", "target", target)
				panic(fmt.Errorf("target is nil"))
			}
			qualifiedName = targetElement.QualifiedName()
		}

		owner.(*transition).target = qualifiedName
		return owner
	}
}

func Effect[T context.Context](fn func(hsm HSM[T], event AnyEvent), maybeName ...string) Partial {
	return func(model *Builder, stack []elements.Element) elements.Element {
		owner := find(stack, kinds.Transition)
		if owner == nil {
			slog.Error("effect must be called within a Transition")
			panic(fmt.Errorf("effect must be called within a Transition"))
		}
		name := ".entry"
		if len(maybeName) > 0 {
			name = maybeName[0]
		}
		behavior := &behavior{
			element: element{kind: kinds.Behavior, qualifiedName: path.Join(owner.QualifiedName(), name)},
			action:  fn,
		}
		model.elements[behavior.QualifiedName()] = behavior
		owner.(*transition).effect = behavior.QualifiedName()
		return owner
	}
}

func Guard[T context.Context](fn func(hsm HSM[T], event AnyEvent) bool, maybeName ...string) Partial {
	return func(model *Builder, stack []elements.Element) elements.Element {
		owner := find(stack, kinds.Transition)
		if owner == nil {
			slog.Error("guard must be called within a Transition")
			panic(fmt.Errorf("guard must be called within a Transition"))
		}
		name := ".guard"
		if len(maybeName) > 0 {
			name = maybeName[0]
		}
		constraint := &constraint[T]{
			element:    element{kind: kinds.Constraint, qualifiedName: path.Join(owner.QualifiedName(), name)},
			expression: fn,
		}
		model.elements[constraint.QualifiedName()] = constraint
		owner.(*transition).guard = constraint.QualifiedName()
		return owner
	}
}

func Initial[T interface{ string | Partial }](elementOrName T, partialElements ...Partial) Partial {
	return func(model *Builder, stack []elements.Element) elements.Element {
		owner := find(stack, kinds.StateMachine, kinds.State)
		if owner == nil {
			slog.Error("initial must be called within a Model or State")
			panic(fmt.Errorf("initial must be called within a Model or State"))
		}
		initial := &vertex{
			element: element{kind: kinds.Initial, qualifiedName: path.Join(owner.QualifiedName(), ".initial")},
		}
		if model.elements[initial.QualifiedName()] != nil {
			panic(fmt.Errorf("Initial state already exists"))
		}
		var target string
		switch any(elementOrName).(type) {
		case string:
			if !path.IsAbs(any(elementOrName).(string)) {
				target = path.Join(owner.QualifiedName(), any(elementOrName).(string))
			} else {
				target = any(elementOrName).(string)
			}
		case Partial:
			any(elementOrName).(Partial)(model, stack)
			if maybeTarget := find(stack, kinds.State, kinds.Choice); maybeTarget != nil {
				target = maybeTarget.QualifiedName()
			}
		}
		model.elements[initial.QualifiedName()] = initial
		stack = append(stack, initial)
		transition := Transition(append(partialElements, Target(target), Source(initial.QualifiedName()))...)(model, stack)
		if transition.(elements.Transition).Guard() != "" {
			panic(fmt.Errorf("guards are not allowed on initial states"))
		}
		return transition
	}
}

func Choice(partialElements ...Partial) Partial {
	return func(builder *Builder, stack []elements.Element) elements.Element {
		owner := find(stack, kinds.State, kinds.Transition)
		if owner == nil {
			slog.Error("choice must be called within a State or Transition")
			panic(fmt.Errorf("choice must be called within a State or Transition"))
		} else if kinds.IsKind(owner.Kind(), kinds.Transition) {
			source := owner.(*transition).source
			owner = builder.elements[source]
			if owner == nil {
				slog.Error("missing source", "id", source)
				panic(fmt.Errorf("missing source %s", source))
			}
		}
		qualifiedName := path.Join(owner.QualifiedName(), fmt.Sprintf("choice_%d", len(builder.elements)))
		element := &vertex{
			element: element{kind: kinds.Choice, qualifiedName: qualifiedName},
		}
		builder.elements[qualifiedName] = element
		stack = append(stack, element)
		apply(builder, stack, partialElements...)
		if len(element.transitions) == 0 {
			slog.Error("choice must have at least one transition")
			panic(fmt.Errorf("choice must have at least one transition"))
		}
		if defaultTransition := get[elements.Transition](&builder.model, element.transitions[len(element.transitions)-1]); defaultTransition != nil {
			if defaultTransition.Guard() != "" {
				panic(fmt.Errorf("the last transition of a choice state cannot have a guard"))
			}
		}
		return element
	}
}

func Entry[T context.Context](fn func(hsm HSM[T], event AnyEvent), maybeName ...string) Partial {
	return func(model *Builder, stack []elements.Element) elements.Element {
		owner := find(stack, kinds.State)
		if owner == nil {
			slog.Error("entry must be called within a State")
			panic(fmt.Errorf("entry must be called within a State"))
		}
		name := ".entry"
		if len(maybeName) > 0 {
			name = maybeName[0]
		}
		element := &behavior{
			element: element{kind: kinds.Behavior, qualifiedName: path.Join(owner.QualifiedName(), name)},
			action:  fn,
		}
		model.elements[element.QualifiedName()] = element
		owner.(*state).entry = element.QualifiedName()
		return element
	}
}

func Activity[T context.Context](fn func(hsm HSM[T], event AnyEvent), maybeName ...string) Partial {
	return func(model *Builder, stack []elements.Element) elements.Element {
		owner := find(stack, kinds.State)
		if owner == nil {
			slog.Error("activity must be called within a State")
			panic(fmt.Errorf("activity must be called within a State"))
		}
		name := ".activity"
		if len(maybeName) > 0 {
			name = maybeName[0]
		}
		element := &behavior{
			element: element{kind: kinds.Concurrent, qualifiedName: path.Join(owner.QualifiedName(), name)},
			action:  fn,
		}
		model.elements[element.QualifiedName()] = element
		owner.(*state).activity = element.QualifiedName()
		return element
	}
}

func Exit[T context.Context](fn func(hsm HSM[T], event AnyEvent), maybeName ...string) Partial {
	return func(model *Builder, stack []elements.Element) elements.Element {
		owner := find(stack, kinds.State)
		if owner == nil {
			slog.Error("exit must be called within a State")
			panic(fmt.Errorf("exit must be called within a State"))
		}
		name := ".exit"
		if len(maybeName) > 0 {
			name = maybeName[0]
		}
		element := &behavior{
			element: element{kind: kinds.Behavior, qualifiedName: path.Join(owner.QualifiedName(), name)},
			action:  fn,
		}
		model.elements[element.QualifiedName()] = element
		owner.(*state).exit = element.QualifiedName()
		return element
	}
}

func Trigger(events ...string) Partial {
	return func(model *Builder, stack []elements.Element) elements.Element {
		owner := find(stack, kinds.Transition)
		if owner == nil {
			slog.Error("trigger must be called within a Transition")
			panic(fmt.Errorf("trigger must be called within a Transition"))
		}
		for _, name := range events {
			owner.(*transition).events[name] = &event{
				element: element{kind: kinds.Event, qualifiedName: name},
			}
		}
		return owner
	}
}

func After(duration time.Duration) Partial {
	return func(builder *Builder, stack []elements.Element) elements.Element {
		owner := find(stack, kinds.Transition)
		if owner == nil {
			panic(fmt.Errorf("after must be called within a Transition"))
		}
		name := fmt.Sprintf("after-%d", len(owner.(*transition).events))
		owner.(*transition).events[name] = &event{
			element: element{kind: kinds.TimeEvent, qualifiedName: name},
			data:    duration,
		}
		return owner
	}
}

func Event(name string, maybeData ...any) *event {
	var data any
	if len(maybeData) > 0 {
		data = maybeData[0]
	}
	return &event{
		element: element{kind: kinds.Event, qualifiedName: name},
		data:    data,
	}
}

func Final(name string) Partial {
	return func(builder *Builder, stack []elements.Element) elements.Element {
		panic("not implemented")
	}
}

type hsm struct {
	behavior
	state  elements.Element
	model  *model
	active map[string]*active
	mutex  *sync.Mutex
}

type HSM[T context.Context] struct {
	context.Context
	*hsm
	storage T
}

type Context[T context.Context] HSM[T]

var contextKey = unique.Make("context")

func New[T context.Context](ctx T, model *model) *HSM[T] {
	hsm := &HSM[T]{
		hsm: &hsm{
			behavior: behavior{
				element: element{
					kind:          kinds.StateMachine,
					qualifiedName: model.QualifiedName(),
				},
			},
			model:  model,
			active: map[string]*active{},
			mutex:  &sync.Mutex{},
		},
		storage: ctx,
	}
	all, ok := ctx.Value(contextKey).(*sync.Map)
	if !ok {
		all = &sync.Map{}
	}
	all.Store(hsm, struct{}{})
	hsm.Context = context.WithValue(ctx, contextKey, all)
	hsm.action = func(ctx HSM[T], event AnyEvent) {
		hsm.mutex.Lock()
		defer hsm.mutex.Unlock()
		hsm.state = hsm.initial(hsm, event)
	}
	hsm.execute(&hsm.behavior, nil)
	return hsm
}

func (hsm *HSM[T]) State() string {
	if hsm == nil {
		return ""
	}
	if hsm.state == nil {
		return ""
	}
	return hsm.state.QualifiedName()
}

func (hsm *HSM[T]) Storage() T {
	return hsm.storage
}

func (hsm *HSM[T]) enter(element elements.Element, event AnyEvent) elements.Element {
	if hsm == nil {
		return nil
	}
	kind := element.Kind()
	if kinds.IsKind(kind, kinds.State) {
		state := element.(*state)
		if entry := get[*behavior](hsm.model, state.entry); entry != nil {
			hsm.execute(entry, event)
		}
		activity := get[*behavior](hsm.model, state.activity)
		if activity != nil {
			hsm.execute(activity, event)
		}
		for _, qualifiedName := range state.transitions {
			if element := get[*transition](hsm.model, qualifiedName); element != nil {
				for _, event := range element.Events() {
					switch event.Kind() {
					case kinds.TimeEvent:
						current, ok := hsm.active[event.QualifiedName()]
						if !ok {
							current = &active{}
							hsm.active[event.QualifiedName()] = current
						}
						var ctx context.Context
						ctx, current.cancel = context.WithCancel(hsm)
						current.channel = make(chan struct{})
						go func(ctx context.Context, channel chan struct{}) {
							timer := time.NewTimer(event.Data().(time.Duration))
							defer timer.Stop()
							defer close(channel)
							for {
								select {
								case <-ctx.Done():
									return
								case <-hsm.Done():
									return
								case <-timer.C:
									go hsm.Dispatch(event)
									return
								}
							}
						}(ctx, current.channel)
					}
				}
			}
		}
		// TODO: completion transitions
		// go func() {
		// 	if activity != nil {
		// 		hsm.wait(activity)
		// 	}
		// 	hsm.mutex.Lock()
		// 	defer hsm.mutex.Unlock()
		// 	slog.Info("activity completed", "activity", activity.QualifiedName())
		// }()
		// if len(defaultEntry) > 0 && defaultEntry[0] {
		// return hsm.enter(element, event, true)
		// }
		return element
	} else if kinds.IsKind(kind, kinds.Choice) {
		for _, qualifiedName := range element.(*vertex).transitions {
			if element := get[*transition](hsm.model, qualifiedName); element != nil {
				hsm.enter(element, event)
			}
		}
	}

	return nil
}

func (hsm *HSM[T]) initial(element elements.Element, event AnyEvent) elements.Element {
	if element == nil || hsm == nil {
		return nil
	}
	var qualifiedName string
	if element.QualifiedName() == "/" {
		qualifiedName = "/.initial"
	} else {
		qualifiedName = element.QualifiedName() + "/.initial"
	}
	if initial := get[*vertex](hsm.model, qualifiedName); initial != nil {
		if len(initial.transitions) > 0 {
			if transition := get[*transition](hsm.model, initial.transitions[0]); transition != nil {
				return hsm.transition(element, transition, event)
			}
		}
	}
	return element
}

func (hsm *HSM[T]) exit(element elements.Element, event AnyEvent) {
	if element == nil || hsm == nil {
		return
	}
	if state, ok := element.(*state); ok {
		for _, qualifiedName := range state.transitions {
			if element := get[*transition](hsm.model, qualifiedName); element != nil {
				for _, event := range element.Events() {
					switch event.Kind() {
					case kinds.TimeEvent:
						active, ok := hsm.active[event.QualifiedName()]
						if ok {
							active.cancel()
							<-active.channel
						}
					}
				}
			}
		}
		if activity := get[*behavior](hsm.model, state.activity); activity != nil {
			hsm.terminate(activity)
		}
		if exit := get[*behavior](hsm.model, state.exit); exit != nil {
			hsm.execute(exit, event)
		}
	}

}

func (hsm *HSM[T]) execute(element elements.Behavior, event AnyEvent) elements.Behavior {
	if hsm == nil || element == nil {
		return nil
	}
	action := element.Action()
	if action == nil {
		return nil
	}
	if !kinds.IsKind(element.Kind(), kinds.Concurrent) {
		if action, ok := action.(func(ctx HSM[T], event AnyEvent)); ok {
			action(HSM[T]{
				Context: hsm.Context,
				hsm:     hsm.hsm,
				storage: hsm.storage,
			}, event)
		}
		return element
	}
	current, ok := hsm.active[element.QualifiedName()]
	if current == nil || !ok {
		current = &active{}
		hsm.active[element.QualifiedName()] = current
	}
	var ctx context.Context
	ctx, current.cancel = context.WithCancel(hsm)
	current.channel = make(chan struct{})
	go func(channel chan struct{}) {
		if action, ok := action.(func(ctx HSM[T], event AnyEvent)); ok {
			action(HSM[T]{
				Context: ctx,
				hsm:     hsm.hsm,
				storage: hsm.storage,
			}, event)
		}
		close(channel)
	}(current.channel)
	return element
}

func (hsm *HSM[T]) evaluate(guard *constraint[T], event AnyEvent) bool {
	if hsm == nil || guard == nil || guard.expression == nil {
		return true
	}
	return guard.expression(
		HSM[T]{
			Context: hsm.Context,
			hsm:     hsm.hsm,
			storage: hsm.storage,
		},
		event,
	)
}

func (hsm *HSM[T]) transition(current elements.Element, transition *transition, event AnyEvent) elements.Element {
	if hsm == nil {
		return nil
	}
	path, ok := transition.paths[current.QualifiedName()]
	if !ok {
		return nil
	}
	for _, exiting := range path.exit {
		current, ok = hsm.model.elements[exiting]
		if !ok {
			return nil
		}
		hsm.exit(current, event)
	}
	if effect := get[elements.Behavior](hsm.model, transition.effect); effect != nil {
		hsm.execute(effect, event)
	}
	if kinds.IsKind(transition.kind, kinds.Internal) {
		return current
	}
	for _, entering := range path.enter {
		current, ok = hsm.model.elements[entering]
		if !ok {
			return nil
		}
		hsm.enter(current, event)
	}
	current, ok = hsm.model.elements[transition.target]
	if !ok {
		return nil
	}
	return hsm.initial(current, event)
}

func (hsm *HSM[T]) terminate(element *behavior) {
	if hsm == nil || element == nil {
		return
	}
	active, ok := hsm.active[element.QualifiedName()]
	if !ok {
		return
	}
	active.cancel()
	<-active.channel

}

func (hsm *HSM[T]) Dispatch(event AnyEvent) bool {
	if hsm == nil {
		return false
	}
	hsm.mutex.Lock()
	defer hsm.mutex.Unlock()
	if hsm.state == nil {
		return false
	}
	state := hsm.state.QualifiedName()
	for state != "/" {
		source := get[elements.Vertex](hsm.model, state)
		if source == nil {
			return false
		}
		for _, transitionQualifiedName := range source.Transitions() {
			transition := get[*transition](hsm.model, transitionQualifiedName)
			if transition == nil {
				continue
			}
			if _, ok := transition.Events()[event.Name()]; !ok {
				continue
			}
			if guard := get[*constraint[T]](hsm.model, transition.Guard()); guard != nil {
				if !hsm.evaluate(guard, event) {
					continue
				}
			}
			hsm.state = hsm.transition(hsm.state, transition, event)
			return true
		}
		state = source.Owner()
	}
	return false
}

func (sm *HSM[T]) DispatchAll(event AnyEvent) {
	active, ok := sm.Value(contextKey).(*sync.Map)
	if !ok {
		return
	}
	active.Range(func(value any, _ any) bool {
		sm, ok := value.(elements.StateMachine)
		if !ok {
			return true
		}
		go sm.Dispatch(event)
		return true
	})
}
