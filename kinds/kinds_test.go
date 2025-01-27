package kinds_test

import (
	"testing"

	"github.com/stateforward/go-hsm/kinds"
)

func TestKinds(t *testing.T) {
	if !kinds.IsKind(kinds.StateMachine, kinds.Behavior) {
		t.Errorf("StateMachine should be a Behavior")
	}
	if kinds.IsKind(kinds.StateMachine, kinds.Vertex) {
		t.Errorf("StateMachine should not be a Vertex")
	}
	if !kinds.IsKind(kinds.State, kinds.Vertex) {
		t.Errorf("State should be a Vertex")
	}
	if kinds.IsKind(kinds.State, kinds.Behavior) {
		t.Errorf("State should not be a Behavior")
	}
	if !kinds.IsKind(kinds.Choice, kinds.Pseudostate) {
		t.Errorf("Choice should be a PseudoState")
	}
	if !kinds.IsKind(kinds.Choice, kinds.Vertex) {
		t.Errorf("Choice should not be a Vertex")
	}
}
