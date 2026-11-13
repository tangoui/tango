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
