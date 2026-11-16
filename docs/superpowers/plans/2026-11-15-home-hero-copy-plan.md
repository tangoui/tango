# Refresh Homepage Hero Copy Implementation Plan

> 历史实施计划，产品改动已合入 main。下方原步骤用于追溯，不是当前待办，也不是重放提交或推送的指令。

**归档复核：** 2026-11-16，源码基线 `116b9e63`。

| 项目 | 已核实状态 | 依据 |
| --- | --- | --- |
| 首页标题和说明文案 | 已实现、已提交并集成 | `b42c75b`；基线 `site/index.md` 已包含目标文案 |
| 后续品牌大小写 | 已另行更新，不属于本计划改动 | `10fcb8d` |
| 集成与同步 | `b42c75b` 已进入本地 main 和本次复核时已获取的 origin/main 历史 | 本地提交祖先关系核对 |
| 当时的渲染检查、站点构建 | 原始执行记录未在本次归档复核 | 不因源码存在就补记为测试通过 |

归档保留原始目标及流程；没有重新执行产品修改、站点构建、截图、rebase 或 push。

**Goal:** Reposition the Tango homepage hero around a visually polished, high-performance Go GUI while preserving the existing desktop/mobile and WebView/self-drawn implementation details.

**Architecture:** Update the VitePress homepage frontmatter only. Keep the existing hero layout, actions, logo, and downstream feature cards unchanged so the copy change remains localized and reversible.

**Tech Stack:** VitePress Markdown frontmatter, Chinese product copy, Git.

**Spec:** Approved in conversation on 2026-11-15: lead with “高颜值、高性能” and retain the current technical differentiators in supporting copy.

## Global Constraints

- Modify only `site/index.md` for the product copy.
- Preserve the existing hero actions, image, warning, and feature cards.
- Keep claims grounded in the existing implementation: Go GUI, GPU rendering, WebView integration, desktop and mobile support.
- Do not stage or alter pre-existing untracked files.

## Review Focus

- Hero title communicates both visual quality and performance at a glance.
- Supporting text still names the desktop/mobile scope and WebView plus Go-rendered UI.
- The new copy fits the existing hero layout without adding extra lines that disturb the screenshot's hierarchy.
- Existing CTA labels and links remain unchanged.
- Markdown frontmatter remains valid.

### Task 1: Refresh the homepage hero copy

**Files:**
- Modify: `site/index.md:4-10`

**Interfaces:**
- Consumes: Existing VitePress home layout and Tango implementation claims.
- Produces: Updated `titleTemplate`, hero `text`, and hero `tagline`.

- **原步骤 1: Update the hero title and tagline**
  Use a concise title centered on “高颜值、高性能的 Go GUI”，with the supporting line explaining that Tango builds desktop and mobile apps, supports system WebView and Go custom rendering in one window, and uses the system WebView engine without bundling a browser engine.

- **原步骤 2: Review the rendered copy**
  Run a focused diff and inspect the frontmatter to verify only the intended three copy fields changed.

- **原步骤 3: Run the site validation**
  Run the repository's site build command from `site/package.json`; expected result is exit code 0.

- **原步骤 4: Commit the focused change**
  Stage only `site/index.md` and commit with message `docs(site): sharpen homepage hero positioning`.

- **原步骤 5: Sync and push**
  原流程包括 fetch、必要的 rebase 和 push。最终提交已进入上述分支历史；这里仅保留历史同步目标，不作为当前推送指令。
