# EvaluationContext Usage

The `EvaluationContext` now implements the standard `context.Context` interface, allowing you to pass it to any function that expects a context while also maintaining your evaluation attributes.

## Basic Usage

```go
// Simple usage with default background context
ctx := evaluation.NewEvaluationContext(map[string]string{
    "user_id": "123",
    "region": "us-west",
})

err := client.GetFig("my-config", &myConfig, ctx)
```

## With Timeouts

```go
// Create a context with timeout
baseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// Create an evaluation context that respects the timeout
ctx := evaluation.NewEvaluationContextWithContext(baseCtx, map[string]string{
    "user_id": "123",
    "region": "us-west",
})

// This will fail if it takes longer than 5 seconds
err := client.GetFig("my-config", &myConfig, ctx)
```

## With Cancellation

```go
// Create a cancellable context
baseCtx, cancel := context.WithCancel(context.Background())

ctx := evaluation.NewEvaluationContextWithContext(baseCtx, map[string]string{
    "user_id": "123",
})

// Start a goroutine that might cancel
go func() {
    <-someSignal
    cancel()
}()

err := client.GetFig("my-config", &myConfig, ctx)
// Will return immediately if cancel() is called
```

## With Request Context (HTTP Handlers)

```go
func MyHandler(w http.ResponseWriter, r *http.Request) {
    // Use the request's context for proper cancellation handling
    ctx := evaluation.NewEvaluationContextWithContext(r.Context(), map[string]string{
        "user_id": getUserID(r),
        "region": getRegion(r),
    })
    
    var config MyConfig
    if err := client.GetFig("feature-flags", &config, ctx); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    // Use config...
}
```

## Benefits

1. **Single context parameter**: No need to pass both a `context.Context` and an `*EvaluationContext`
2. **Timeout/cancellation support**: Automatically propagates to all underlying operations (decryption, API calls, etc.)
3. **Request-scoped tracing**: Works with OpenTelemetry and other tracing frameworks that use `context.Context`
4. **Standard Go patterns**: Follows Go's standard library conventions
