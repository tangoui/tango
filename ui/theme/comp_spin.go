package theme

// SpinToken styles Spin and Loading (antd es/spin/style, EP loading, shadcn spinner.tsx).
type SpinToken struct {
	Alias
}

func deriveSpin(t *Theme) SpinToken {
	return SpinToken{}
}
