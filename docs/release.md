# 发布流程

> 文档站：[发布 Tango 自己](https://tangoui.github.io/tango/guide/packaging#发布-tango-自己) · 本文是维护者的发布手册。

Tango 一次发布包含三样东西，它们**使用同一个版本号**：

| 产物 | 位置 | 版本标记 |
|---|---|---|
| Go 模块 `github.com/tangoui/tango` | 仓库根目录 | git tag `vX.Y.Z` |
| 命令行 `tan`（独立 Go 模块 `github.com/tangoui/tango/cmd/tan`） | `cmd/tan` | git tag `cmd/tan/vX.Y.Z`；另附各平台二进制 |
| npm 包 `@tangoui/runtime` | `runtime-js` | `runtime-js/package.json` 的 `version` = `X.Y.Z` |

## 版本号规则

遵循 [语义化版本 2.0.0](https://semver.org/lang/zh-CN/)：

- **0.x 阶段**（当前）：`0.MINOR.0` 可以包含不兼容的公开 API 变化，须在 CHANGELOG 的「变更」/「移除」里写明迁移方法；`0.MINOR.PATCH` 只做向后兼容的修复。
- **1.0 之后**：不兼容变化只进 MAJOR；新增功能进 MINOR；修复进 PATCH。
- **预发布**：`vX.Y.Z-rc.N`、`-beta.N`。发布工作流把它们标为 GitHub pre-release，npm 用 `next` 标签发布（不影响 `latest`），不部署文档站。
- **三者同步**：即使某次只改了其中一部分，也一起升到同一个版本，方便用户对应。`tan init` 生成的项目引用同版本的 `@tangoui/runtime`。
- **Go 模块路径**：进入 v2 时模块路径要加 `/v2` 后缀（Go 的规则），届时另行规划。
- `tan version` 的版本号来自构建时的 `-ldflags "-X main.version=X.Y.Z"`；`go install …@vX.Y.Z` 安装的则取模块版本。

## API 稳定性

Go 模块的公开包分两层：

| 层 | 包 | 不兼容变化 |
|---|---|---|
| 稳定 | 根包 `tango`、`ipc`、`bind`、`asset`、`jsapi`、`sys/...`、`service/...`（`service/updater` 除外）、`i18n`、`automation` 等其余非 internal 包 | 默认不允许；确有必要时走下面的 `api-break` 流程 |
| 实验性 | `ui/...`、`uihost`、`mobile/...`、`service/updater`、`examples/...` | 允许，CI 只给警告；仍建议在 CHANGELOG 里写一句 |

CI 的 `API` 工作流（`.github/workflows/api.yml`）用 `scripts/apidiff.sh` 比较 PR 的基准提交（推送时是推送前的 `main`）和当前代码，在 linux、darwin、windows 三个平台上各导出一次 API。稳定包出现不兼容变化时检查失败。本地可以先跑：

```bash
bash scripts/apidiff.sh            # 对比 origin/main（含未提交的改动）
bash scripts/apidiff.sh v0.1.0     # 对比某个标签
```

改稳定包时，优先用兼容的写法：新增函数或选项结构体、用可变参数追加选项、旧名字保留并标 `// Deprecated:`。确实要破坏兼容时：

1. 给 PR 加 `api-break` 标签（直接推送 `main` 时在提交说明里写 `[api-break]`），检查转为警告；
2. 在 CHANGELOG「未发布」的「变更」或「移除」里写明迁移方法；
3. 这类变化只能进下一个 `0.MINOR.0`（1.0 之后只进 MAJOR）。

注意 apidiff 视为不兼容的情况比"编译不过"更宽：例如给函数追加可变参数（把函数当值使用的代码会编译失败）、结构体加了切片字段而不再可比较、导出常量的值变化。

要把实验性包升为稳定，把它从 `scripts/apidiff.sh` 的 `EXPERIMENTAL_RE` 和上表中移除。

## 发布前提（一次性）

- 仓库 Settings → Pages → Source 选 **GitHub Actions**（文档站由 `release.yml` 的 `docs` 作业部署）。
- Settings → Secrets and variables → Actions 添加 **`NPM_TOKEN`**：npm 上 `@tangoui` 组织的 Automation（或细粒度、可发布 `@tangoui/runtime`）令牌。没有它时 `npm` 作业会给出警告并跳过，其余照常。
- Settings → Code security 打开 **Private vulnerability reporting**（SECURITY.md 依赖它）。
- `.github/workflows/ci.yml` 里还有早期的 `release` / `publish` 作业（同样由 `v*` 标签触发、创建草稿 Release）。启用 `release.yml` 后应删除这两个作业，否则同一标签会出现两个草稿 Release。

## 发布清单

以发布 `0.2.0` 为例。

### 1. 准备

- [ ] `main` 上 CI 全绿（lint、三平台 test 与 smoke、mobile 作业）。
- [ ] 在 macOS 上手动跑 `bash scripts/smoke.sh` 和 `go run ./examples/showcase -smoke`；有条件的话在 Windows、Linux 实机上各跑一遍，并在发布说明里如实写明验证范围。
- [ ] `CHANGELOG.md`：把「未发布」里的内容移到新的 `## [0.2.0] - YYYY-MM-DD` 一节，更新文末的比较链接。
- [ ] `runtime-js/package.json` 的 `version` 改成 `0.2.0`，在 `runtime-js` 里 `npm install --package-lock-only` 同步 lock 文件，`npm test` 通过，提交更新后的 `runtimejs/tango.iife.js`（如有变化）。
- [ ] `cmd/tan` 模板里引用的 `@tangoui/runtime` 版本与本次一致（`tan init` 生成的 package.json）。
- [ ] 文档：新功能有对应页面；实验性功能有标注。`cd site && npm run docs:build && npm run docs:links` 通过。

### 2. 打根模块标签

```bash
git switch main && git pull
git tag -a v0.2.0 -m "Tango v0.2.0"
git push origin v0.2.0
```

推送 `v0.2.0` 触发 `.github/workflows/release.yml`：

| 作业 | 做什么 |
|---|---|
| `verify` | 标签格式；`runtime-js/package.json` 版本与标签一致；CHANGELOG 有 `## [0.2.0]`；`go vet` / `go test`（根模块与 cmd/tan）；`runtime-js` 的 `npm test` |
| `tan` | `CGO_ENABLED=0` 交叉编译 `tan`（darwin / linux / windows × amd64 / arm64），打成 `tan_0.2.0_<os>_<arch>.tar.gz` / `.zip`，附 `tan_0.2.0_checksums.txt` |
| `npm` | 有 `NPM_TOKEN` 时 `npm publish --provenance --access public` 发布 `@tangoui/runtime@0.2.0`（`prepublishOnly` 会先构建并测试）；该版本已存在则跳过 |
| `release` | 从 CHANGELOG 取本版本一节作为说明，附加 GitHub 自动生成的 PR 列表，创建**草稿** Release 并上传二进制；Release 已存在时只更新附件和说明 |
| `docs` | 正式版本才执行：在 Linux 上生成组件截图、构建文档站（含 API 参考）、检查死链，部署到 GitHub Pages |

### 3. 检查并发布 Release

- [ ] 打开 GitHub Releases 里的草稿，核对说明、附件和校验和，按 **Publish release**。
- [ ] npm：`npm view @tangoui/runtime@0.2.0`，页面上应显示 provenance。
- [ ] 文档站已更新。

### 4. 打 cmd/tan 子模块标签

`cmd/tan` 是独立模块。`cmd/tan/go.mod` 必须依赖一个可下载的根模块版本（正式标签或伪版本），不能用 `v0.0.0` 占位，也不能带 `replace`。本地开发由仓库根目录的 `go.work` 连接两个模块，无需把本地路径写进发布模块。根模块发布后，再把 CLI 的依赖更新到该版本：

```bash
cd cmd/tan
GOWORK=off GOPROXY=https://proxy.golang.org go get github.com/tangoui/tango@v0.2.0
GOWORK=off go mod tidy
cd ../..
git commit -s -am "cmd/tan: require github.com/tangoui/tango v0.2.0"
git tag -a cmd/tan/v0.2.0 -m "tan v0.2.0"
git push origin main cmd/tan/v0.2.0
```

- [ ] 验证：在一个空目录里 `go install github.com/tangoui/tango/cmd/tan@v0.2.0 && tan version`。
- [ ] 本地开发继续由仓库根目录的 `go.work` 把两个模块连在一起，不需要恢复 `replace`。

普通开发提交也必须保持 CLI 可独立构建：在 `cmd/tan` 目录执行 `GOWORK=off go build -mod=readonly .`。CI 会执行同一检查，并拒绝 CLI 模块里的 `replace`；发布校验也会将它视为错误。没有正式版本时，可以依赖已经推送、能被 Go 模块代理下载的根模块伪版本。本地修复尚未推送时，远程的 `go install …@latest` 仍会读取旧的 `go.mod`；可先在本地 `cmd/tan` 目录执行 `go install .`。

> `cmd/tan/v*` 标签不匹配 `v*` 触发条件，不会再跑一遍发布工作流。

### 5. 收尾

- [ ] `CHANGELOG.md` 顶部保留空的「未发布」一节。
- [ ] 需要的话在讨论区 / 社交渠道发布公告，链接到 Release 和文档站。

## 出问题时

- **`verify` 失败**：修正后删除并重打标签（`git push --delete origin v0.2.0 && git tag -d v0.2.0`），前提是 npm 还没发布这个版本。
- **npm 已发布但有问题**：npm 上的版本号不能复用。发布 `0.2.1` 修复，并 `npm deprecate @tangoui/runtime@0.2.0 "<原因>"`。Go 模块同理：已推送的标签不要移动（Go 校验和数据库已经记录），用新补丁版本修复，必要时在 `go.mod` 里用 `retract` 撤回。
- **只想重建附件**：Actions → Release → Run workflow，填已有标签；这时不会发布 npm、不会部署文档。
