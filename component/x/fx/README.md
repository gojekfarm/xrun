# Fx bridge

`New(lifecycle, shutdowner, components...)` registers one Fx hook pair and returns a `Runner`.
It accepts zero components and rejects nil components, including typed nil values.
The bridge starts each `xrun.Component.Run` with a private context that lives until failure or Fx stop.
The Fx startup context is used only to decide whether starting is still allowed.

`OnStart` launches components but provides no readiness handshake.
Acquire dependencies and bind listeners in an earlier `OnStart` hook when startup must report those failures synchronously.
Register that hook before the bridge so its `OnStop` runs after the bridge's `OnStop`.
See [ExampleNew](example_test.go) for a listener with this ordering and a finite `fx.StopTimeout`.

The first unexpected component result cancels peers and requests Fx shutdown with exit code 1.
Returning nil before cancellation is an unexpected exit.
Cancellation-only errors after requested cancellation are normal; other errors, including errors joined with cancellation, are retained by `Runner.Err()` and returned by the stop hook.
`Runner.Done()` closes only when every `Run` and any shutdown notification has finished.
`Runner.Active()` is false when no component work remains, including when Fx skips the bridge hook during startup rollback.

Fx stop uses its hook context as the join deadline.
If a component ignores cancellation, the hook returns a descriptive deadline error, but the component can continue running.
Fx may return its own context deadline error before surfacing the hook's error; inspect `Runner.Err()` for component failures and `Runner.Active()` for remaining work.
Fx may also invoke later stop hooks after an earlier hook error, so resource cleanup must check `Runner.Active()` before closing resources used by a component.
If the component finishes after cleanup was skipped, the application must arrange a later cleanup or terminate the process.

`Run` implementations must join their own child work before returning.
For an HTTP server, drain active handlers before closing their database or cache clients; joining only `Serve` does not establish that handlers are finished.
The bridge cannot force noncooperative code to stop or provide readiness for a generic `Component`.
