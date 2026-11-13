//go:build race

package highlight

// raceFactor 放大测试中的墙钟时限（-race 构建慢 5~20 倍）。
const raceFactor = 10

// raceEnabled reports a -race build.
const raceEnabled = true
