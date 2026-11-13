//go:build !race

package monarch

// raceFactor 见 race.go；普通构建不放大时限。
const raceFactor = 1
