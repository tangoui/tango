# Go-drawn UI

Create the app with `tango.New`. Call `uihost.Open` from `app.OnReady`, using a draw function `func(*ui.Context)`. Save all three results `runner, surface, err`. Use the surface for window operations and the runner to schedule changes to UI state. The generated `ui` template shows a complete component tree with layout.

Give every dynamic list row a stable key based on the record's identity. Pass the checkbox label before the pointer to its value.

```go
type task struct {
	id   int
	text string
	done bool
}

func drawTasks(c *ui.Context, tasks []task) {
	for i := range tasks {
		item := &tasks[i]
		ui.Key(c, item.id, func() {
			ui.Checkbox(c, item.text, &item.done)
		})
	}
}
```

Keep state writes in the UI callback or schedule them with `runner.Post(func() { /* update state */ })` when work completes in a goroutine. The post requests a redraw. Use `c.WakeAt(nextUpdate)` to schedule a future frame, such as the next second of a countdown. Compute remaining time from a deadline so delayed frames do not slow the countdown. Keep disk and network work out of the drawing path where it could block a frame.

Use `app.Post` for work that needs the platform main thread and does not itself request a native UI redraw. [Verification](verify.md)

From your app directory, run `go run ./.agents/skills/tango/examples/native-tray-timer`. Use `-data-dir` to choose where it saves its checklist. Copy the complete example folder, including `timer.png`, to adapt it in your app. [Native tray timer source](../examples/native-tray-timer/main.go)
