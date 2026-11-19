package main

// ttyGuard: Windows consoles have no foreground process group to lose.
type ttyGuard struct{}

func newTTYGuard() *ttyGuard { return &ttyGuard{} }
func (*ttyGuard) check()     {}
