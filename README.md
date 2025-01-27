# go-hsm [![PkgGoDev](https://pkg.go.dev/badge/github.com/stateforward/go-hsm)](https://pkg.go.dev/github.com/stateforward/go-hsm)

> **Warning**
> This package is currently in alpha stage. While it has test coverage, the API is subject to breaking changes between minor versions until we reach v1.0.0. Please pin your dependencies to specific versions.

Package go-hsm provides a powerful hierarchical state machine (HSM) implementation for Go. State machines help manage complex application states and transitions in a clear, maintainable way.

## Key Features

- Hierarchical state organization
- Entry, exit, and activity actions for states
- Guard conditions and transition effects
- Event-driven transitions
- Time-based transitions
- Context and storage support
- Choice pseudo-states for dynamic branching
- Concurrent state execution
- Event queuing with completion event priority
- Multiple state machine instances with broadcast support


## Roadmap

Current and planned features:

- [x] Event-driven transitions
- [x] Time-based transitions with delays
- [x] Hierarchical state nesting
- [x] Entry/exit/activity actions
- [x] Guard conditions
- [x] Transition effects
- [x] Custom storage context
- [x] Choice pseudo-states
- [x] Event broadcasting
- [x] Concurrent activities
- [ ] Scheduled transitions (at specific dates/times)
   ```go
   hsm.Transition(
       hsm.At(time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)),
       hsm.Source("active"),
       hsm.Target("expired")
   )
   ```
- [ ] History support (shallow and deep)
   ```go
   hsm.State("parent",
       hsm.History(), // Shallow history
       hsm.DeepHistory(), // Deep history
       hsm.State("child1"),
       hsm.State("child2")
   )
   ```


## Basic Usage

Here's a simple example of creating and using a state machine:

```go
model := hsm.Model(
    hsm.State("foo"),
    hsm.State("bar"),
    hsm.Transition(
        hsm.Trigger("moveToBar"),
        hsm.Source("foo"),
        hsm.Target("bar")
    ),
    hsm.Initial("foo")
)

sm := hsm.New(context.Background(), &model)
sm.Dispatch(hsm.Event("moveToBar"))
```

## State Actions

States can have three types of actions:

1. **Entry Actions** - Executed when entering a state
2. **Activity Actions** - Long-running activities while in a state
3. **Exit Actions** - Executed when leaving a state

```go
hsm.State("active",
    // Entry action - runs once when state is entered
    hsm.Entry(func(ctx hsm.Context[*storage], event hsm.Event) {
        log.Println("Entering active state")
    }),
    
    // Activity action - can run long-running operations
    hsm.Activity(func(ctx hsm.Context[*storage], event hsm.Event) {
        // Will be cancelled when state is exited
        select {
        case <-ctx.Done():
            return
        case <-time.After(time.Second):
            log.Println("Activity tick")
        }
    }),
    
    // Exit action - runs when leaving the state
    hsm.Exit(func(ctx hsm.Context[*storage], event hsm.Event) {
        log.Println("Exiting active state")
    })
)
```

## Choice States

Choice pseudo-states allow dynamic branching based on conditions:

```go
hsm.State("processing",
    hsm.Transition(
        hsm.Trigger("decide"),
        hsm.Target(
            hsm.Choice(
                // First matching guard wins
                hsm.Transition(
                    hsm.Target("approved"),
                    hsm.Guard(func(ctx hsm.Context[*storage], event hsm.Event) bool {
                        return ctx.Storage().score > 700
                    }),
                ),
                // Default transition (no guard)
                hsm.Transition(
                    hsm.Target("rejected")
                ),
            ),
        ),
    ),
)
```

## Event Broadcasting

Multiple state machine instances can receive broadcasted events:

```go
sm1 := hsm.New(context.Background(), &model)
sm2 := hsm.New(context.Background(), &model)

// Dispatch event to all state machines
sm1.DispatchAll(hsm.NewEvent("globalEvent"))
```

## Transitions

Transitions define how states change in response to events. They can include:

1. **Guards** - Conditions that must be true for the transition to occur
2. **Effects** - Zero side effect actions performed during the transition

```go
hsm.Transition(
    hsm.Trigger("submit"),
    hsm.Source("draft"),
    hsm.Target("review"),
    // Guard condition
    hsm.Guard(func(ctx hsm.Context[*storage], event AnyEvent) bool {
        return ctx.Storage().isValid
    }),
    // Transition effect
    hsm.Effect(func(ctx hsm.Context[*storage], event AnyEvent) {
        log.Println("Transitioning from draft to review")
    })
)
```

## Hierarchical States

States can be nested to create hierarchical state machines:

```go
model := hsm.Model(
    hsm.State("operational",
        hsm.State("idle"),
        hsm.State("running"),
        hsm.Initial("idle"),
        hsm.Transition(
            hsm.Trigger("start"),
            hsm.Source("idle"),
            hsm.Target("running")
        )
    ),
    hsm.State("maintenance"),
    hsm.Initial("operational")
)
```

## Time-Based Transitions

You can create transitions that occur after a time delay:

```go
hsm.Transition(
    hsm.After(time.Second * 30),
    hsm.Source("active"),
    hsm.Target("timeout")
)
```

## Custom Storage

State machines can maintain custom storage that's accessible in actions and guards:

```go
type MyStorage struct {
    context.Context
    counter int
    status  string
}

model := // ... define model ...
sm := hsm.New(&MyStorage{
    Context: context.Background(),
    counter: 0,
    status:  "ready",
}, &model)
```

## Installation

```bash
go get github.com/stateforward/go-hsm
```

## License

MIT - See LICENSE file

## Contributing

Contributions are welcome! Please ensure:
- Tests are included
- Code is well documented
- Changes maintain backward compatibility
