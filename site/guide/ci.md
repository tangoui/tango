# CI 与冒烟测试

Tango 的 CI 配置在 `.github/workflows/ci.yml`。冒烟测试入口 `scripts/smoke.sh`（macOS / Linux）和 `scripts/smoke.ps1`（Windows）本地和 CI 共用，CI 上失败的步骤都能在本机用同一条命令复现。

## 本地运行

```bash
bash scripts/smoke.sh                          # 本平台全部桌面冒烟程序
bash scripts/smoke.sh list                     # 列出程序名
bash scripts/smoke.sh desktop hello smokeext   # 只跑指定的几个
bash scripts/smoke.sh ios                      # examples/mobile-hello 在 iPhone 模拟器里运行
bash scripts/smoke.sh android                  # 构建 APK，装到已连接的设备 / 模拟器上运行
pwsh scripts/smoke.ps1                         # Windows
```

每个程序弹出真实窗口几秒，然后自己退出；脚本先 `go build` 再运行，给每个程序设硬超时，最后打印汇总。有任何 FAIL、TIMEOUT 或构建失败，退出码非零。输出在 `build/smoke/`：每个程序一个 `.log`，`summary.txt` 是汇总，开了截图时有 `.png`。

很多示例自带 `-smoke` 模式，可以单独跑：

```bash
go run ./examples/hello -smoke
go run ./examples/runtime-demo -smoke
go run ./examples/showcase -smoke
```

### 环境变量

| 变量 | 作用 | 默认 |
|---|---|---|
| `SMOKE_OUT` | 日志、截图、汇总目录 | `build/smoke` |
| `SMOKE_TIMEOUT` | 每个程序的硬超时（秒） | 120 |
| `SMOKE_SCREENSHOT=1` | 运行中截一张整屏图（macOS 需要终端有屏幕录制权限） | 关 |
| `TANGO_SMOKE_TIMEOUT_SCALE` | 冒烟程序**自己的**超时乘以这个倍数，给慢机器用；CI 设为 3 | 1 |
| `TANGO_SMOKE_SKIP` | notify-demo 跳过的检查组：`clipboard`、`hotkey`、`notify`、`shortcut` | 不跳过 |
| `SMOKE_IOS_DEVICE` | 模拟器名称或 UDID | 第一个可用 iPhone |

## CI 作业

触发：推送到 `main`、`v*` 标签、所有 PR、手动运行。

| 作业 | 环境 | 内容 |
|---|---|---|
| `lint` | ubuntu | `gofmt -l`；`go vet ./...`（根模块与 `cmd/tan`）；`CGO_ENABLED=0` 下对 windows/linux/darwin × amd64/arm64 交叉 vet；`runtime-js` 的 `npm ci && npm test`，并检查提交的 IIFE 与源码构建结果一致 |
| `test` | ubuntu / macos / windows | `go test -race ./...` 与 `cmd/tan` 的测试 |
| `smoke` | ubuntu / macos / windows | 真实窗口冒烟。Linux 装 GTK、WebKitGTK、AppIndicator、xvfb，在 `xvfb-run` 下运行 |
| `mobile-ios` | macos | 构建 mobile-hello，在 iPhone 模拟器里运行并截图 |
| `mobile-android` | ubuntu | 打 APK 并上传为产物；在 API 34 模拟器里运行（目前 `continue-on-error`） |

发布相关的工作流（`release.yml`）见 [docs/release.md](https://github.com/tangoui/tango/blob/main/docs/release.md)。

::: info
CI 的 Windows、Linux 冒烟在 GitHub 托管 runner 上跑真实窗口（Linux 在虚拟显示里），这是目前这两个平台的主要验证手段；在真实桌面环境中的系统性验证还没有做。
:::

## 给你自己的应用做 CI

- 桌面构建不需要 cgo，一台 Linux runner 就能交叉编译三个平台：`tan build -platform darwin/arm64,windows/amd64,linux/amd64`（`.dmg` 只能在 macOS 上做，签名也需要对应平台的工具）。
- 给应用加一个 `-smoke` 模式：页面跑完自检后用事件告诉 Go，Go 打印 PASS/FAIL 并以退出码表示结果，参考 `examples/showcase`。
- Linux runner 上需要 `libgtk-3-0 libwebkit2gtk-4.1-0 xvfb`，用 `xvfb-run -a` 运行；无头会话没有通知服务。
