package hsm_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stateforward/go-hsm"
	"github.com/stateforward/go-hsm/pkg/plantuml"
	"github.com/stateforward/go-hsm/pkg/tests"
)

type Trace struct {
	sync  []string
	async []string
}

func (t *Trace) reset() {
	t.sync = []string{}
	t.async = []string{}
}

func (t *Trace) matches(expected Trace) bool {
	if expected.sync != nil && !slices.Equal(t.sync, expected.sync) {
		return false
	}
	if expected.async != nil && !slices.Equal(t.async, expected.async) {
		return false
	}
	return true
}

func (t *Trace) contains(expected Trace) bool {
	if expected.sync != nil && slices.ContainsFunc(t.sync, func(s string) bool {
		return slices.Contains(expected.sync, s)
	}) {
		return true
	}
	if expected.async != nil && slices.ContainsFunc(t.async, func(s string) bool {
		return slices.Contains(expected.async, s)
	}) {
		return true
	}
	return false
}

type storage struct {
	context.Context
	foo int
}

func TestHSM(t *testing.T) {
	trace := &Trace{}

	mockAction := func(name string, async bool) func(hsm hsm.HSM[*storage], event hsm.AnyEvent) {
		return func(hsm hsm.HSM[*storage], event hsm.AnyEvent) {
			if async {
				trace.async = append(trace.async, name)
			} else {
				trace.sync = append(trace.sync, name)
			}
		}
	}
	model := hsm.Model(
		hsm.State("s",
			hsm.Entry(mockAction("s.entry", false)),
			hsm.Activity(mockAction("s.activity", true)),
			hsm.Exit(mockAction("s.exit", false)),
			hsm.State("s1",
				hsm.State("s11",
					hsm.Entry(mockAction("s11.entry", false)),
					hsm.Activity(mockAction("s11.activity", true)),
					hsm.Exit(mockAction("s11.exit", false)),
				),
				hsm.Initial("s11", hsm.Effect(mockAction("s1.initial.effect", false))),
				hsm.Exit(mockAction("s1.exit", false)),
				hsm.Entry(mockAction("s1.entry", false)),
				hsm.Activity(mockAction("s1.activity", true)),
				hsm.Transition(hsm.Trigger("I"), hsm.Effect(mockAction("s1.I.transition.effect", false))),
				hsm.Transition(hsm.Trigger("A"), hsm.Target("/s/s1"), hsm.Effect(mockAction("s1.A.transition.effect", false))),
			),
			hsm.Transition(hsm.Trigger("D"), hsm.Source("/s/s1/s11"), hsm.Target("/s/s1"), hsm.Effect(mockAction("s11.D.transition.effect", false)), hsm.Guard(
				func(hsm hsm.HSM[*storage], event hsm.AnyEvent) bool {
					check := hsm.Storage().foo == 1
					hsm.Storage().foo = 0
					return check
				},
			)),
			hsm.Initial("s1/s11", hsm.Effect(mockAction("s.initial.effect", false))),
			hsm.State("s2",
				hsm.Entry(mockAction("s2.entry", false)),
				hsm.Activity(mockAction("s2.activity", true)),
				hsm.Exit(mockAction("s2.exit", false)),
				hsm.State("s21",
					hsm.State("s211",
						hsm.Entry(mockAction("s211.entry", false)),
						hsm.Activity(mockAction("s211.activity", true)),
						hsm.Exit(mockAction("s211.exit", false)),
						hsm.Transition(hsm.Trigger("G"), hsm.Target("/s/s1/s11"), hsm.Effect(mockAction("s211.G.transition.effect", false))),
					),
					hsm.Initial("s211", hsm.Effect(mockAction("s21.initial.effect", false))),
					hsm.Entry(mockAction("s21.entry", false)),
					hsm.Activity(mockAction("s21.activity", true)),
					hsm.Exit(mockAction("s21.exit", false)),
					hsm.Transition(hsm.Trigger("A"), hsm.Target("/s/s2/s21")), // self transition
				),
				hsm.Initial("s21/s211", hsm.Effect(mockAction("s2.initial.effect", false))),
				hsm.Transition(hsm.Trigger("C"), hsm.Target("/s/s1"), hsm.Effect(mockAction("s2.C.transition.effect", false))),
			),
		),
		hsm.Initial("s/s2", hsm.Effect(mockAction("initial.effect", false))),
		hsm.Transition(hsm.Trigger("D"), hsm.Source("/s/s1"), hsm.Target("/s"), hsm.Effect(mockAction("s1.D.transition.effect", false)), hsm.Guard(
			func(hsm hsm.HSM[*storage], event hsm.AnyEvent) bool {
				check := hsm.Storage().foo == 0
				hsm.Storage().foo++
				return check
			},
		)),
		hsm.Transition(hsm.Trigger("C"), hsm.Source("/s/s1"), hsm.Target("/s/s2"), hsm.Effect(mockAction("s1.C.transition.effect", false))),
		hsm.Transition(hsm.Trigger("E"), hsm.Source("/s"), hsm.Target("/s/s1/s11"), hsm.Effect(mockAction("s.E.transition.effect", false))),
		hsm.Transition(hsm.Trigger("G"), hsm.Source("/s/s1/s11"), hsm.Target("/s/s2/s21/s211"), hsm.Effect(mockAction("s11.G.transition.effect", false))),
		hsm.Transition(hsm.Trigger("I"), hsm.Source("/s"), hsm.Effect(mockAction("s.I.transition.effect", false)), hsm.Guard(
			func(hsm hsm.HSM[*storage], event hsm.AnyEvent) bool {
				check := hsm.Storage().foo == 0
				hsm.Storage().foo = 1
				return check
			},
		)),
		hsm.Transition(hsm.After(time.Second), hsm.Source("/s/s2/s21/s211"), hsm.Target("/s/s1/s11")),
		hsm.Transition(hsm.Trigger("H"), hsm.Source("/s/s1/s11"), hsm.Target(
			hsm.Choice(
				hsm.Transition(hsm.Target("/s/s1"), hsm.Guard(
					func(hsm hsm.HSM[*storage], event hsm.AnyEvent) bool {
						return hsm.Storage().foo == 0
					},
				)),
				hsm.Transition(hsm.Target("/s/s2")),
			),
		)),
	)
	fmt.Println(plantuml.Generate(&model))
	sm := hsm.New(&storage{
		Context: context.Background(),
		foo:     0,
	}, &model)
	tests.Run(t, sm)

	if sm.State() != "/s/s2/s21/s211" {
		t.Error("Initial state is not /s/s2/s21/s211", "state", sm.State())
		return
	}
	if !trace.matches(Trace{
		sync: []string{"initial.effect", "s.entry", "s2.entry", "s2.initial.effect", "s21.entry", "s211.entry"},
	}) {
		t.Fatal("Trace is not correct", "trace", trace)
	}

	trace.reset()
	if !sm.Dispatch(hsm.Event("G")) {
		t.Fatal("event not handled")
	}

	if sm.State() != "/s/s1/s11" {
		t.Fatal("state is not correct", "state", sm.State())
	}
	if !trace.matches(Trace{
		sync: []string{"s211.exit", "s21.exit", "s2.exit", "s211.G.transition.effect", "s1.entry", "s11.entry"},
	}) {
		t.Fatal("trace is not correct", "trace", trace)
	}
	trace.reset()
	if !sm.Dispatch(hsm.Event("I")) {
		t.Fatal("event not handled")
	}
	if sm.State() != "/s/s1/s11" {
		t.Fatal("state is not correct", "state", sm.State())
	}
	if !trace.matches(Trace{
		sync: []string{"s1.I.transition.effect"},
	}) {
		t.Fatal("transition actions are not correct", "trace", trace)
	}
	trace.reset()
	if !sm.Dispatch(hsm.Event("A")) {
		t.Error("event A not handled")
		return
	}
	if sm.State() != "/s/s1/s11" {
		t.Error("state is not correct", "state", sm.State())
		return
	}
	if !trace.matches(Trace{
		sync: []string{"s11.exit", "s1.exit", "s1.A.transition.effect", "s1.entry", "s1.initial.effect", "s11.entry"},
	}) {
		t.Error("transition actions are not correct", "trace", trace)
	}
	trace.reset()
	if !sm.Dispatch(hsm.Event("D")) {
		t.Error("event D not handled")
		return
	}
	if sm.State() != "/s/s1/s11" {
		t.Error("state is not correct", "state", sm.State())
		return
	}
	if !trace.matches(Trace{
		sync: []string{"s11.exit", "s1.exit", "s1.D.transition.effect", "s.initial.effect", "s1.entry", "s11.entry"},
	}) {
		t.Error("transition actions are not correct", "trace", trace)
	}
	trace.reset()
	if !sm.Dispatch(hsm.Event("D")) {
		t.Error("event D not handled")
		return
	}
	if sm.State() != "/s/s1/s11" {
		t.Error("state is not correct", "state", sm.State())
		return
	}
	if !trace.matches(Trace{
		sync: []string{"s11.exit", "s11.D.transition.effect", "s1.initial.effect", "s11.entry"},
	}) {
		t.Error("transition actions are not correct", "trace", trace)
	}
	trace.reset()
	if !sm.Dispatch(hsm.Event("C")) {
		t.Error("event C not handled")
		return
	}
	if sm.State() != "/s/s2/s21/s211" {
		t.Error("state is not correct", "state", sm.State())
		return
	}
	if !trace.matches(Trace{
		sync: []string{"s11.exit", "s1.exit", "s1.C.transition.effect", "s2.entry", "s2.initial.effect", "s21.entry", "s211.entry"},
	}) {
		t.Error("transition actions are not correct", "trace", trace)
	}
	trace.reset()
	if !sm.Dispatch(hsm.Event("E")) {
		t.Error("event E not handled")
		return
	}
	if sm.State() != "/s/s1/s11" {
		t.Error("state is not correct", "state", sm.State())
		return
	}
	if !trace.matches(Trace{
		sync: []string{"s211.exit", "s21.exit", "s2.exit", "s.E.transition.effect", "s1.entry", "s11.entry"},
	}) {
		t.Error("transition actions are not correct", "trace", trace)
	}
	trace.reset()
	if !sm.Dispatch(hsm.Event("E")) {
		t.Error("event E not handled")
		return
	}
	if sm.State() != "/s/s1/s11" {
		t.Error("state is not correct", "state", sm.State())
		return
	}
	if !trace.matches(Trace{
		sync: []string{"s11.exit", "s1.exit", "s.E.transition.effect", "s1.entry", "s11.entry"},
	}) {
		t.Error("transition actions are not correct", "trace", trace)
	}
	trace.reset()
	if !sm.Dispatch(hsm.Event("G")) {
		t.Error("event G not handled")
		return
	}
	if sm.State() != "/s/s2/s21/s211" {
		t.Error("state is not correct", "state", sm.State())
		return
	}
	if !trace.matches(Trace{
		sync: []string{"s11.exit", "s1.exit", "s11.G.transition.effect", "s2.entry", "s21.entry", "s211.entry"},
	}) {
		t.Error("transition actions are not correct", "trace", trace)
	}
	trace.reset()
	if !sm.Dispatch(hsm.Event("I")) {
		t.Error("event I not handled")
		return
	}
	if sm.State() != "/s/s2/s21/s211" {
		t.Error("state is not correct", "state", sm.State())
		return
	}
	if !trace.matches(Trace{
		sync: []string{"s.I.transition.effect"},
	}) {
		t.Error("transition actions are not correct", "trace", trace)
	}
	trace.reset()
	time.Sleep(2 * time.Second)
	if sm.State() != "/s/s1/s11" {
		t.Error("state is not correct", "state", sm.State())
	}

}

//	func TestHSMDispatchAll(t *testing.T) {
//		model := hsm.Model(
//			hsm.State("foo"),
//			hsm.State("bar"),
//			hsm.Transition(hsm.Trigger("foo"), hsm.Source("foo"), hsm.Target("bar")),
//			hsm.Transition(hsm.Trigger("bar"), hsm.Source("bar"), hsm.Target("foo")),
//		)
//		sm1 := hsm.New(context.Background(), &model)
//		sm2 := hsm.New(context.Background(), &model)
//		sm2.DispatchAll(hsm.Event{Type: "foo"})
//		if
func noBehavior(hsm hsm.HSM[context.Context], event hsm.AnyEvent) {

}

// }
var benchModel = hsm.Model(
	hsm.State("foo",
		hsm.Entry(noBehavior),
		hsm.Exit(noBehavior),
	),
	hsm.State("bar",
		hsm.Entry(noBehavior),
		hsm.Exit(noBehavior),
	),
	hsm.Transition(
		hsm.Trigger("foo"),
		hsm.Source("foo"),
		hsm.Target("bar"),
		hsm.Effect(noBehavior),
	),
	hsm.Transition(
		hsm.Trigger("bar"),
		hsm.Source("bar"),
		hsm.Target("foo"),
		hsm.Effect(noBehavior),
	),
	hsm.Initial("foo", hsm.Effect(noBehavior)),
)
var benchSM = hsm.New(context.Background(), &benchModel)

func BenchmarkHSM(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchSM.Dispatch(hsm.Event("foo"))
		benchSM.Dispatch(hsm.Event("bar"))
	}
}

func nonHSMLogic() func(event hsm.AnyEvent) bool {
	type state int
	const (
		foo state = iota
		bar
	)
	currentState := foo
	// Simulating entry/exit actions as no-ops to match HSM version
	fooEntry := func(event hsm.AnyEvent) {}
	fooExit := func(event hsm.AnyEvent) {}
	barEntry := func(event hsm.AnyEvent) {}
	barExit := func(event hsm.AnyEvent) {}
	initialEffect := func(event hsm.AnyEvent) {}

	// Transition effects as no-ops
	fooToBarEffect := func(event hsm.AnyEvent) {}
	barToFooEffect := func(event hsm.AnyEvent) {}

	// Event handling
	handleEvent := func(event hsm.AnyEvent) bool {
		switch currentState {
		case foo:
			if event.Name() == "foo" {
				fooExit(event)
				fooToBarEffect(event)
				currentState = bar
				barEntry(event)
				return true
			}
		case bar:
			if event.Name() == "bar" {
				barExit(event)
				barToFooEffect(event)
				currentState = foo
				fooEntry(event)
				return true
			}
		}
		return false
	}

	// Initial transition
	initialEffect(nil)
	fooEntry(nil)
	return handleEvent
}

func BenchmarkNonHSM(b *testing.B) {
	handler := nonHSMLogic()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !handler(hsm.Event("foo")) {
			b.Fatal("event not handled")
		}
		if !handler(hsm.Event("bar")) {
			b.Fatal("event not handled")
		}
	}
}
