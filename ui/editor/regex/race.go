//go:build race

package regex

// raceFactor 放大墙钟时限（-race 构建）：竞态检测器使执行慢 5~20 倍，且有不可预测的停顿，
// 正常输入也会触及按普通构建标定的时限。生产构建不带 -race，时限不变（norace.go）。
const raceFactor = 10
