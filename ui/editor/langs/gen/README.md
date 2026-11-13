# langs 数据导出脚本

`export.mjs` 把 `monaco-editor@0.56.0` 的语言数据转成 JSON，供 `ui/editor/langs` 用
`//go:embed` 内置。只在升级 Monaco 数据时运行，运行时不依赖 JS。

## 重新生成

```sh
cd ui/editor/langs/gen
npm install          # 安装 package.json 固定的 monaco-editor 0.56.0 与 esbuild（node_modules 已被 git 忽略）
node export.mjs      # 需要联网：下载 microsoft/monaco-editor 标签 v0.56.0 的源码包取测试用例
cd ../../../.. && go test ./ui/editor/langs/
```

另有 `export_colors.mjs`：导出 Monaco 颜色注册表的默认颜色，生成
`ui/editor/theme/colors_gen.go`（不需要联网）：

```sh
cd ui/editor/langs/gen
node export_colors.mjs   # 打印计数与抽查值，写 ../../theme/colors_gen.go 并 gofmt
cd ../../../.. && go test ./ui/editor/theme/
```

## 产物

| 路径 | 内容 |
| --- | --- |
| `../data/<id>.json` | 每个注册的语言 id 一份：`{"registrationOrder", "registration", "language", "conf"}` |
| `../data/themes.json` | `vs`、`vs-dark`、`hc-black`、`hc-light` 的 `{base, inherit, rules, colors}` |
| `../testdata/tokenize/<id>.json` | 官方分词用例 `[[{"line", "tokens": [{"startIndex", "type"}]}]]`，按 `testTokenization` 的主语言 id 归组 |

- `RegExp` 一律写成 `{"$regex": source, "flags": flags}`；函数、循环引用、非有限数字、
  非普通对象会让脚本报错并列出路径，不会静默丢弃。
- `registrationOrder` 是 Monaco 的注册顺序（`register.all.js` 顺序，最后是 `vue`），
  Go 侧用它复现 Monaco「后注册者胜出」的文件关联规则（如 `.pp` 归 ruby）。
- `vue` 是额外注册（与 MinCode `monaco-runtime.js` 一致），复用 html 的语法与配置。

## 实现要点

- 语言：用 esbuild 打包 `register.all.js`，把 `_.contribution.js` 换成收集
  `registerLanguage()` 参数的桩、把 `editor.api.js` 换成只导出 `languages.IndentAction`
  （取自 Monaco 自己的 `languageConfiguration.js`）的桩，再逐个调用 `loader()`。
  这样 c/cpp、systemverilog/verilog、freemarker2 七个变体都按 Monaco 实际的加载结果导出。
- 主题：esbuild 打包 `themes.js`，颜色键由 Monaco 的颜色注册表解析成字符串。
- 用例：下载标签源码包到临时目录，esbuild 打包每个 `*.test.ts`，把
  `../test/testRunner` 换成收集函数；测试文件若导入其他模块则报错。同一主语言 id
  的多个文件合并，逐字节相同的用例只保留一份。
- 脚本最后打印计数，与 `EXPECTED` 不符或有任何错误时退出码为 1 且不写任何文件。

## 默认颜色（export_colors.mjs）

- 用 esbuild 打包 `esm/vs` 下所有调用 `registerColor(` 的模块（编辑器各贡献点的颜色都在内，
  不含定义它的 `colorUtils.js`），以 banner 注入一个极小的 DOM 垫片（`window`、`document`
  等若干全局量换成吸收一切访问的 Proxy；颜色注册表本身不用 DOM）。
- 对 `vs`、`vs-dark`、`hc-black`、`hc-light` 各建一个与 Monaco `StandaloneTheme` 解析规则相同的
  主题对象（先查基础主题自己的 `colors`，再按注册表默认值解析引用与 `transparent`、`lighten`、
  `darken`、`oneOf`、`lessProminent`、`ifDefinedThenElse` 等变换），逐键得到最终颜色。
- 产物 `colors_gen.go` 含三张表：`defaultColors`（四个基础主题下的最终颜色）、`defaultUnset`
  （某主题下没有颜色的键，即 null 默认值）、`colorDefaults`（注册表默认值表达式本身，Go 端
  用它对带自定义颜色的主题重新解析，见 `theme/colors.go`）。
- 默认值引用了未注册且不在主题颜色里的键、出现未知变换、或模块数/颜色数与 `EXPECTED` 不符时，
  脚本列出原因、不写文件并以退出码 1 结束。`DANGLING` 列出的三个 VS Code 工作台颜色
  （`sideBar.background` 等）在独立版 Monaco 中本就解析为空，按 Monaco 行为保留为引用；
  这个名单本身也会被校验（不再被引用时报错）。
