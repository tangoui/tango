<!-- 感谢贡献！提交前请读 CONTRIBUTING.md。Thanks! Please read CONTRIBUTING.md first. -->

## 改了什么 / What

<!-- 简要说明改动；关联 issue 写 Fixes #123。 -->

## 为什么 / Why

## 怎么验证的 / How it was verified

<!-- 跑了哪些测试、冒烟程序；新增了哪些测试。 -->

- [ ] `gofmt -l .` 无输出，`go vet ./...`、`go test ./...` 通过（根模块与 `cmd/tan`）
- [ ] 改了平台代码：`CGO_ENABLED=0 GOOS={darwin,windows,linux} go vet ./...` 通过
- [ ] 改了后端 / 窗口 / IPC / 系统能力：跑过 `scripts/smoke.sh`（或 `smoke.ps1`）
- [ ] 改了 `runtime-js`：`npm test` 通过，已提交更新后的 `runtimejs/tango.iife.js`

亲自验证过的平台 / Platforms verified by hand：

- [ ] macOS
- [ ] Windows
- [ ] Linux
- [ ] iOS（模拟器 / 真机）
- [ ] Android（模拟器 / 真机）

## 清单 / Checklist

- [ ] 公开 API 有英文注释；相关的 `docs/`、`site/` 页面已更新
- [ ] 用户可见的变化已写进 `CHANGELOG.md` 的「未发布」
- [ ] 没有复制其他项目的代码或文档文字（引入第三方代码已在 issue 中说明并保留许可证）
- [ ] 每个提交都有 `Signed-off-by`（`git commit -s`，见 DCO）
