package theme

// CommandToken styles Command (shadcn command.tsx).
type CommandToken struct {
	Alias
}

func deriveCommand(t *Theme) CommandToken {
	return CommandToken{}
}
