# 安全策略 / Security Policy

## 支持的版本

Tango 处于 0.x 阶段，只为**最新发布的 0.x 小版本**提供安全修复；修复会以新的补丁版本发布，不回移到更早的小版本。

| 版本 | 是否支持 |
|---|---|
| 最新的 0.x.y | ✓ |
| 更早的版本 | ✗ |

## 报告漏洞

**请不要在公开 issue、讨论区或 PR 里报告安全漏洞。**

请使用 GitHub 的私密漏洞报告：打开仓库的 [Security → Report a vulnerability](https://github.com/tangoui/tango/security/advisories/new)，填写：

- 受影响的组件（如根包、`ipc`、`jsapi`、某个 `sys/` 包、`cmd/tan`、`@tangoui/runtime`）和版本 / 提交；
- 平台（macOS / Windows / Linux / iOS / Android 及系统版本、WebView 版本）；
- 复现步骤或概念验证代码，以及你认为的影响。

我们会在 **5 个工作日内**确认收到，并在评估后告诉你预计的修复时间。修复发布时会通过 GitHub Security Advisory 公开说明，并在你同意的情况下致谢。

## 范围

特别关心的问题包括：

- 页面绕过 `jsapi` 能力白名单，或调用到未开启的系统能力；
- 来自非受信 origin（第三方页面、iframe）的消息被 Go 侧接受；
- `shell` 的协议白名单、`service/fileserver` 的目录限制被绕过；
- `service/updater` 接受了未正确签名、被降级或被篡改的更新；
- `tan` 在构建、签名、打包过程中泄露密钥或证书密码。

应用自己通过 `bind` / `ipc` 暴露给页面的方法不在框架的责任范围内，但如果你发现框架的默认行为容易让应用犯错，也欢迎报告。

---

## English

Please **do not** report security vulnerabilities in public issues, discussions or pull requests. Use GitHub's private vulnerability reporting instead: [Security → Report a vulnerability](https://github.com/tangoui/tango/security/advisories/new). Include the affected component and version, the platform, reproduction steps and the impact you expect. We acknowledge reports within 5 business days. Only the latest 0.x minor release receives security fixes.
