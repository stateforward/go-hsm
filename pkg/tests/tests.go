package tests

import (
	"context"
	"testing"

	"github.com/stateforward/go-hsm"
)

type Test struct {
	name string
	fn   func(t *testing.T, sm *hsm.HSM[context.Context])
}

func Run[T context.Context](t *testing.T, sm *hsm.HSM[T], maybeEvents ...map[string]hsm.Event) {
}
