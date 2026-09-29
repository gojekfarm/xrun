# Fx bridge

`Module()` supervises `xrun.Component` values in the Fx value group `xrun`.
Provide concrete instances with standard Fx naming and add those same instances to the group.
The [named public/admin example](example_test.go) uses `fx.In` and `fx.Out` to do this without caller-owned listener or runner variables.
Installing `Module()` twice is an Fx construction error.

An ordinary `xrun.Component` starts asynchronously and has no readiness signal.
Components that need synchronous startup checks can implement `PreparedComponent` with `Prepare(ctx)`, `Run(ctx)`, and `Close(ctx)`.
The bridge calls all `Prepare` methods before launching any `Run`, then calls `Close` in reverse preparation order after all `Run` calls have returned.
`Prepare` must undo its own partial acquisitions when it fails; the bridge closes only earlier successful preparations.
Fx value-group order is unspecified, so no public/admin ordering is promised.
Components that require ordered preparation should be combined into one owner or use an explicit earlier Fx lifecycle hook.

The first unexpected `Run` result cancels peers and requests Fx shutdown with exit code 1.
Returning nil before cancellation is unexpected.
Cancellation-only errors after requested cancellation are normal; other errors, including failures joined with cancellation, are retained.
`Runner.Err()` and the low-level `New` API remain available to callers that need direct error inspection.
`Runner.Done()` closes after all launched `Run` calls and any shutdown notification complete.
`Runner.Active()` also covers preparation and cleanup, and is false if Fx skips the bridge hook during startup rollback.

Use a finite `fx.StopTimeout` and make `Run` and `Close` respect cancellation.
If `Run` ignores cancellation, Fx may return its own deadline error before the bridge can report an unjoined component; `Close` is withheld while that component may still use its resources.
If `Close` ignores its context, Fx cannot forcibly finish cleanup within the deadline.
On failed or expired preparation, rollback `Close` receives the startup context, which may already be canceled; callers must design cleanup to release resources in that case or handle a reported cleanup error.
After a stop timeout, a later cleanup or process termination is the application's responsibility.

`Run` must join its own child work before returning.
For an HTTP server, drain active handlers before closing their database or cache clients; joining only `Serve` does not establish that handlers are finished.
The bridge cannot infer generic readiness or force noncooperative code to stop.
