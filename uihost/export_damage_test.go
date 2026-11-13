package uihost

// SetDamageLogf replaces the partial-present log and returns a restore
// function.
func SetDamageLogf(f func(string, ...any)) func() {
	old := damageLogf
	damageLogf = f
	return func() { damageLogf = old }
}
