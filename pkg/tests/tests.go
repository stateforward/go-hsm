package tests

import (
	"testing"

	"github.com/stateforward/go-hsm"
)

type Test struct {
	name string
	fn   func(t *testing.T, sm hsm.HSM)
}

func Run(t *testing.T, sm hsm.HSM, maybeEvents ...map[string]hsm.Event) {
}
