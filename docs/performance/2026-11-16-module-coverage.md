# 全模块覆盖状态

基线：`40ed885ff8b1e0607e11aee04a9551a8a16b03b5`；以下按包逐一记录，实测不等于已完成优化。

第一批已合并本地 main（`e662739`）；第二批基于该提交继续。场景数保留首轮口径，新增场景及证据见 [第二批报告](2026-11-16-modules-batch2.md)。

第二批已提交 `902c52d`，第三批继续优化绑定与资源路径，见 [第三批报告](2026-11-16-modules-batch3.md)。本批分配回归已通过，低负载耗时对照尚未取得；没有把分配改进等同于已证明加速。

| 包 | 实测场景数 | 本批状态 |
| --- | ---: | --- |
| `/` | 0 | 未测性能：缺少代表负载与基准 |
| `/asset` | 1 | 第三批路径分配优化：普通测试/race/vet 通过；HTTP/生成输入对照通过，耗时尚未验证 |
| `/automation` | 0 | 未测性能：缺少代表负载与基准 |
| `/bind` | 3 | 第三批参数切片/Context/空调用分配优化：普通测试/race/vet、旧实现对照通过；耗时尚未验证 |
| `/examples/a11y-demo` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/app-demo` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/dock-demo` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/duplex` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/duplex/services` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/duplex-ios` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/duplex-mobile` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/editor-demo` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/hello` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/i18n-demo` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/ledger` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/mobile-hello` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/mobile-ui` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/notify-demo` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/print-demo` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/runtime-demo` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/showcase` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/system-demo` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/sysx-demo` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/terminal` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/terminal-workspace` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/ui-gallery` | 2 | 已建立/运行基线；尚未完成定点优化 |
| `/examples/ui-gallery/gallery` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/ui-hotreload` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/ui-window` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/update-demo` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/webview-demo` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/webview-embed` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/examples/webview-overlay` | 0 | 未测性能：示例启动/实际交互场景待补 |
| `/i18n` | 2 | 第二批 Canonical/Match 定点优化：五轮 A/B、模块测试/race/vet 通过；消息格式化等热点待评估 |
| `/internal/apitest` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/automation` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/cmd/apitxt` | 0 | 未测性能：构建/CLI 场景待补 |
| `/internal/cmd/icongen` | 0 | 未测性能：构建/CLI 场景待补 |
| `/internal/cmd/shadcngen` | 0 | 未测性能：构建/CLI 场景待补 |
| `/internal/corehook` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/devfeat` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/devhost` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/devhost/host` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/lane` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/platform` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/platform/a11ybridge` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/android` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/darwin` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/darwin/internal/smoke` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/darwin/internal/smokeattach` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/darwin/internal/smokecanvas` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/darwin/internal/smokeext` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/darwin/internal/smokemetal` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/darwin/internal/smoketray` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/fake` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/fake/apphook` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/fake/faketest` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/ios` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/linux` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/mobilecanvas` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/native` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/rolenames` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/platform/windows` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/internal/render/gpu` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/render/gpu/atlas` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/render/gpu/cpuexec` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/render/gpu/d3d11` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/render/gpu/gen` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/render/gpu/gl` | 2 | 已建立/运行基线；尚未完成定点优化 |
| `/internal/render/gpu/gl/nativewindow` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/render/gpu/internal/devkit` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/render/gpu/internal/gpuconf` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/render/gpu/internal/gputest` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/render/gpu/internal/racetest` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/render/gpu/metal` | 23 | 已建立/运行基线；尚未完成定点优化 |
| `/internal/render/gpu/shaders` | 0 | 未测性能：缺少代表负载与基准 |
| `/internal/render/shader/cmd/tanshader` | 0 | 未测性能：构建/CLI 场景待补 |
| `/internal/render/shader/front` | 1 | 已建立/运行基线；尚未完成定点优化 |
| `/internal/render/shader/glsl` | 0 | 未测性能：构建/CLI 场景待补 |
| `/internal/render/shader/hlsl` | 0 | 未测性能：构建/CLI 场景待补 |
| `/internal/render/shader/ir` | 0 | 未测性能：构建/CLI 场景待补 |
| `/internal/render/shader/msl` | 0 | 未测性能：构建/CLI 场景待补 |
| `/internal/render/shader/sl` | 0 | 未测性能：构建/CLI 场景待补 |
| `/ipc` | 4 | 第三批已 profile；追加复用场景零分配，解析结果分配保持独立所有权；本批未改源码 |
| `/jsapi` | 0 | 未测性能：缺少代表负载与基准 |
| `/runtime-js/runtimejs` | 0 | 未测性能：缺少代表负载与基准 |
| `/service` | 0 | 未测性能：缺少代表负载与基准 |
| `/service/fileserver` | 0 | 未测性能：缺少代表负载与基准 |
| `/service/kv` | 3 | 第一批已优化并通过 A/B、完整测试与 race；后续热点未宣称完成 |
| `/service/log` | 0 | 未测性能：缺少代表负载与基准 |
| `/service/paths` | 0 | 未测性能：缺少代表负载与基准 |
| `/service/updater` | 0 | 未测性能：缺少代表负载与基准 |
| `/service/updater/internal/e2e` | 0 | 未测性能：缺少代表负载与基准 |
| `/service/updater/internal/e2e/drillapp` | 0 | 未测性能：缺少代表负载与基准 |
| `/service/updater/updateui` | 0 | 未测性能：缺少代表负载与基准 |
| `/site/tools/apigen` | 0 | 未测性能：缺少代表负载与基准 |
| `/site/tools/compdoc` | 0 | 未测性能：缺少代表负载与基准 |
| `/site/tools/playground` | 0 | 未测性能：缺少代表负载与基准 |
| `/sys/appctl` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/autostart` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/background` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/biometric` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/clipboard` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/device` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/dialog` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/env` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/flyout` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/haptics` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/hotkey` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/internal/capcall` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/jumplist` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/location` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/media` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/menu` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/network` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/notify` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/orientation` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/permission` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/power` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/print` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/pty` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/purchase` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/push` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/recent` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/safearea` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/screen` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/securestore` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/share` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/shell` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/single` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/theme` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/theme/uivars` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/tray` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/widget` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/sys/windowstate` | 0 | 未测性能：需要系统/设备场景；单元测试不代替性能验证 |
| `/ui` | 25 | 已建立/运行基线；尚未完成定点优化 |
| `/ui/a11y` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/chart` | 2 | 第一批已优化并通过 A/B、完整测试与 race；后续热点未宣称完成 |
| `/ui/codehl` | 1 | 已建立/运行基线；尚未完成定点优化 |
| `/ui/editor` | 9 | 已建立/运行基线；尚未完成定点优化 |
| `/ui/editor/buffer` | 6 | 第二批换行归一化优化：五轮 A/B、模块测试/race/vet 通过；其他快照/编辑热点待评估 |
| `/ui/editor/diff` | 5 | 已建立/运行基线；尚未完成定点优化 |
| `/ui/editor/diffview` | 1 | 已建立/运行基线；尚未完成定点优化 |
| `/ui/editor/highlight` | 6 | 已建立/运行基线；尚未完成定点优化 |
| `/ui/editor/keymap` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/editor/langs` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/editor/monarch` | 2 | 语义修复/完整测试/race 通过；固定语料复测比 MyGo 耗时低 61.6% |
| `/ui/editor/regex` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/editor/theme` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/editor/when` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/geom` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/graph` | 6 | 第一批已优化并通过 A/B、完整测试与 race；后续热点未宣称完成 |
| `/ui/icon` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/icon/antd` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/icon/element` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/icon/filetype` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/icon/lucide` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/input` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/internal/flexbox` | 4 | 已建立/运行基线；尚未完成定点优化 |
| `/ui/internal/flexbox/flexstyle` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/layout` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/mobile` | 1 | 已建立/运行基线；尚未完成定点优化 |
| `/ui/paint` | 2 | 已建立/运行基线；尚未完成定点优化 |
| `/ui/terminal` | 6 | 已建立/运行基线；尚未完成定点优化 |
| `/ui/terminal/vt` | 4 | 已建立/运行基线；尚未完成定点优化 |
| `/ui/terminal/vt/internal/widthgen` | 0 | 未测性能：缺少代表负载与基准 |
| `/ui/text` | 15 | 已建立/运行基线；尚未完成定点优化 |
| `/ui/theme` | 2 | 已建立/运行基线；尚未完成定点优化 |
| `/uihost` | 0 | 未测性能：缺少代表负载与基准 |
| `/cmd/tan` | 0 | 未测性能：构建/CLI 场景待补 |
| `/cmd/tan/internal/bindgen` | 0 | 未测性能：构建/CLI 场景待补 |
