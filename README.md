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
- Concurrent state execution
- Event queuing with completion event priority
- Multiple state machine instances with broadcast support
- Event completion tracking with Done channels
- Tracing support for state transitions

## State Machine Concepts

A state machine is a computational model that defines how a system behaves and transitions between different states. Here are key concepts:

- **State**: A condition or situation of the system at a specific moment. For example, a traffic light can be in states like "red", "yellow", or "green".
- **Event**: A trigger that can cause the system to change states. Events can be external (user actions) or internal (timeouts).
- **Transition**: A change from one state to another in response to an event.
- **Guard**: A condition that must be true for a transition to occur.
- **Action**: Code that executes when entering/exiting states or during transitions.
- **Hierarchical States**: States that contain other states, allowing for complex behavior modeling with inheritance.
- **Initial State**: The starting state when the machine begins execution.
- **Final State**: A state indicating the machine has completed its purpose.

### Why Use State Machines?

State machines are particularly useful for:

- Managing complex application flows
- Handling user interactions
- Implementing business processes
- Controlling system behavior
- Modeling game logic
- Managing workflow states

### Learn More

For deeper understanding of state machines:

- [UML State Machine Diagrams](https://www.uml-diagrams.org/state-machine-diagrams.html)
- [Statecharts: A Visual Formalism](https://www.sciencedirect.com/science/article/pii/0167642387900359) - The seminal paper by David Harel
- [State Pattern](https://refactoring.guru/design-patterns/state) - Design pattern implementation
- [State Charts](https://statecharts.dev/) - A comprehensive guide to statecharts

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

Here's a simple example of creating and using a state machine with completion tracking:

```go
// Define your state machine type with embedded HSM
type MyHSM struct {
    hsm.HSM // Required embedded struct
    // Add your custom fields here
    counter int
}

// Create the state machine model
model := hsm.Define(
    "example",
    hsm.State("foo"),
    hsm.State("bar"),
    hsm.Transition(
        hsm.Trigger("moveToBar"),
        hsm.Source("foo"),
        hsm.Target("bar")
    ),
    hsm.Initial("foo")
)

// Create and start the state machine
sm := hsm.Start(context.Background(), &MyHSM{}, &model)

// Create event with completion channel
done := make(chan struct{})
event := hsm.Event{
    Name: "moveToBar",
    Done: done,
}

// Dispatch event and wait for completion
sm.Dispatch(event)
<-done
```

## State Actions

States can have three types of actions:

```go
type MyHSM struct {
    hsm.HSM
    // your custom fields
}

hsm.State("active",
    // Entry action - runs once when state is entered
    hsm.Entry(func(ctx context.Context, hsm *MyHSM, event hsm.Event) {
        log.Println("Entering active state")
    }),

    // Activity action - long-running operation with context
    hsm.Activity(func(ctx context.Context, hsm *MyHSM, event hsm.Event) {
        for {
            select {
            case <-ctx.Done():
                return
            case <-time.After(time.Second):
                log.Println("Activity tick")
            }
        }
    }),

    // Exit action - runs when leaving the state
    hsm.Exit(func(ctx context.Context, hsm *MyHSM, event hsm.Event) {
        log.Println("Exiting active state")
    })
)
```

## Choice States

Choice pseudo-states allow dynamic branching based on conditions:

```go
type MyHSM struct {
    hsm.HSM
    score int
}

hsm.State("processing",
    hsm.Transition(
        hsm.Trigger("decide"),
        hsm.Target(
            hsm.Choice(
                // First matching guard wins
                hsm.Transition(
                    hsm.Target("approved"),
                    hsm.Guard(func(ctx context.Context, hsm *MyHSM, event hsm.Event) bool {
                        return hsm.score > 700
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
sm1 := hsm.Start(context.Background(), &MyHSM{}, &model)
sm2 := hsm.Start(context.Background(), &MyHSM{}, &model)

// Dispatch event to all state machines
hsm.DispatchAll(sm1, hsm.NewEvent("globalEvent"))
```

## Transitions

Transitions define how states change in response to events. They can include:

1. **Guards** - Conditions that must be true for the transition to occur
2. **Effects** - Zero side effect actions performed during the transition

```go
type MyHSM struct {
    hsm.HSM
    // your custom fields
}

hsm.Transition(
    hsm.Trigger("submit"),
    hsm.Source("draft"),
    hsm.Target("review"),
    hsm.Guard(func(ctx context.Context, hsm *MyHSM, event hsm.Event) bool {
        return hsm.IsValid()
    }),
    hsm.Effect(func(ctx context.Context, hsm *MyHSM, event hsm.Event) {
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
    hsm.After(func(ctx context.Context, hsm *MyHSM) time.Duration {
        return time.Second * 30
    }),
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
sm := hsm.Start(context.Background(), &MyStorage{
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
- Signature changes follow the new context+event pattern

## Event Completion Tracking

Events now support completion tracking through Done channels:

```go
// Create event with completion channel
done := make(chan struct{})
event := hsm.Event{
    Name: "process",
    Data: payload,
    Done: done,
}

// Dispatch event
sm.Dispatch(event)

// Wait for processing to complete
select {
case <-done:
    log.Println("Event processing completed")
case <-time.After(time.Second):
    log.Println("Timeout waiting for event processing")
}
```

## Tracing Support

The package now includes tracing capabilities for debugging state transitions:

```go
// Create tracer
trace := func(ctx context.Context, step string, data ...any) (context.Context, func(...any)) {
    log.Printf("[TRACE] %s: %+v", step, data)
    return ctx, func(...any) {}
}

// Start state machine with tracing
sm := hsm.Start(ctx, &MyHSM{}, &model, hsm.Config{
    Trace: trace,
    Id:    "machine-1",
})
```

## Concurrent Activities

Activities now properly handle context cancellation:

```go
hsm.State("processing",
    hsm.Activity(func(ctx context.Context, hsm *MyHSM, event hsm.Event) {
        // Long-running process
        result, err := hsm.ProcessData(ctx, event.Data)
        if err == nil {
            hsm.Dispatch(hsm.Event{
                Name: "processComplete",
                Data: result,
            })
        }
    }),
    hsm.Transition(
        hsm.Trigger("processComplete"),
        hsm.Target("completed")
    )
)
```

## Custom State Machine Types

State machines must embed the `hsm.HSM` struct and can add their own fields:

```go
type MyHSM struct {
    hsm.HSM // Required embedded struct
    counter int
    status  string
}

model := // ... define model ...
sm := hsm.Start(context.Background(), &MyHSM{
    counter: 0,
    status:  "ready",
}, &model)
```

## OpenTelemetry Integration Example

The package's `Trace` interface can be used to integrate with OpenTelemetry. Here's an example implementation:

```go
// Example implementation of hsm.Trace interface using OpenTelemetry
func NewOTelTracer(name string) hsm.Trace {
    provider := initTracerProvider(name)
    tracer := provider.Tracer(name)

    return func(ctx context.Context, step string, data ...any) (context.Context, func(...any)) {
        attrs := []attribute.KeyValue{
            attribute.String("step", step),
            // Add other relevant attributes from data
        }

        ctx, span := tracer.Start(ctx, step, trace.WithAttributes(attrs...))
        return ctx, func(...any) {
            span.End()
        }
    }
}

// Usage with state machine
sm := hsm.Start(ctx, &MyHSM{}, &model, hsm.Config{
    Trace: NewOTelTracer("payment_processor"),
    Id:    "payment-1",
})
```

A complete OpenTelemetry integration example can be found in the examples directory. It shows how to:

- Set up an OTLP exporter
- Configure trace attributes
- Handle context propagation
- Track state machine events and transitions

The `Trace` interface makes it easy to integrate with any tracing system, not just OpenTelemetry. You can implement custom tracers for:

- Logging frameworks
- Metrics systems
- Distributed tracing tools
- Performance monitoring

Example trace output might look like:

```
[payment_processor] [enter] state=/processing event=submit
[payment_processor] [evaluate] guard=validatePayment result=true
[payment_processor] [execute] action=processPayment
[payment_processor] [transition] source=/processing target=/completed
```

### Custom Trace Configuration

You can customize the OpenTelemetry endpoint and other settings:

```go
import (
    "go.opentelemetry.io/otel/exporters/otlp/otlptracehttp"
)

// Configure custom endpoint
otlptracehttp.WithEndpoint("custom.endpoint:4318")
otlptracehttp.WithHeaders(map[string]string{
    "custom-header": "value",
})

// Force flush traces
tracer.Flush()
```

The tracer automatically handles:

- Trace context propagation
- Span attributes
- Error tracking
- Async operation monitoring
