# 移动端扩展：分享扩展与桌面小组件（设计）

状态：Android AppWidget 已实现最小版本（`sys/widget`，见 `docs/mobile.md`「小组件与扩展」）；iOS 分享扩展、iOS 小组件、Android 分享目标留待后续，本文记录评估与设计。

## 1. 现状与约束

- tan 构建 iOS 应用时不用 Xcode 工程：Go c-archive + `mobile/ios/main.m` 用 clang 链接成一个可执行文件，Info.plist 由 tan.json 生成，`codesign` 签名（真机用描述文件）。
- Android 宿主是一个 Gradle 工程（`mobile/android`），只用平台 API，不依赖 androidx，离线构建。
- 扩展/小组件运行在**另一个进程**里（iOS 的 `.appex` 是独立可执行文件；Android 小组件由桌面进程渲染 `RemoteViews`），Go 运行时与页面都不在那里。所以两端都只能是"原生小壳 + 与主应用共享的数据"，不能直接跑应用的 Go 代码或页面。

## 2. Android AppWidget（已实现）

| 部分 | 内容 |
|---|---|
| 宿主 | `TangoWidget extends AppWidgetProvider`；`res/layout/tango_widget.xml`（标题 + 文本）、`res/xml/tango_widget_info.xml`（3×1，可调大小，`updatePeriodMillis=0`）、`res/drawable/tango_widget_background.xml` |
| 清单 | tan.json `permissions` 含 `"widget"` 时，`build_android_app.go` 在 `</application>` 前插入 `<receiver android:name="org.tango.host.TangoWidget" android:exported="false">`（`APPWIDGET_UPDATE` + `android.appwidget.provider` 元数据）。不声明时小组件不会出现在系统的小组件列表里 |
| 数据 | `Device.updateWidget` 写 `SharedPreferences("tango.widget")` 再 `AppWidgetManager.updateAppWidget`；`onUpdate`（添加、重启后）从同一处读，所以应用不运行也显示最后的内容 |
| 交互 | 整个小组件点击 → `getLaunchIntentForPackage` 的 `PendingIntent`（`FLAG_IMMUTABLE`） |
| Go | `widget.Update(app, widget.Content{Title, Text})`；未声明 → `ErrNotConfigured` |

后续可扩展：多种尺寸布局（`RemoteViews` 按尺寸映射，API 31+）、列表（`RemoteViewsService`）、按钮带深链（`PendingIntent` + `ACTION_VIEW` 自定义 scheme，复用 `EventOpenURL`）、配置 Activity。tan.json 若要更细的配置（标题、尺寸），需要在 `config.go` 的 `Android` 里加 `widget` 段（本轮按约定没有动 `config.go`，所以用 `permissions` 里的 `"widget"` 作开关）。

## 3. iOS 分享扩展（未实现）

### 需要什么

1. 一个 Share Extension 目标：`<App>.app/PlugIns/<Name>Share.appex`，内含可执行文件、Info.plist（`CFBundlePackageType=XPC!`，`NSExtension` 字典：`NSExtensionPointIdentifier=com.apple.share-services`，`NSExtensionPrincipalClass`，`NSExtensionAttributes.NSExtensionActivationRule` 声明接受的类型：文本、URL、图片、文件，各自最大数量）。
2. 扩展的可执行文件：ObjC 写一个 `UIViewController` 子类（principal class），从 `self.extensionContext.inputItems` 的 `NSItemProvider` 异步取出内容（`loadItemForTypeIdentifier:`），写到共享容器，然后 `completeRequestReturningItems:`。不链接 Go（扩展内存上限约 120 MB，Go 运行时 + WebKit 不划算；也没有必要）。
3. 共享数据：App Group（`com.apple.security.application-groups` entitlement，主应用与扩展都要），扩展把内容写进 `containerURLForSecurityApplicationGroupIdentifier:` 下的 `inbox/<uuid>/`（`item.json` + 文件）。
4. 交给主应用：
   - 主应用下次到前台时扫描 inbox（`UIApplicationDidBecomeActiveNotification`），逐条作为新事件投递（建议新增 `AppShareReceived`，或复用 `EventOpenFiles` + `EventOpenURL`；前者更好，负载需要 `AppEvent` 新字段，属于契约变更）。
   - 想立即打开主应用：分享扩展没有公开的 `openURL:`（`extensionContext openURL:` 只对 Today/小组件扩展有效），沿响应链调 `UIApplication openURL:` 的做法审核有风险，**不采用**；用户回到主应用时即可看到。
5. 签名：扩展单独签名（它自己的 bundle id `<id>.share`、描述文件、App Group entitlement），再签主应用；模拟器上 ad-hoc 签名也需要把 entitlements 链接进两个可执行文件（与现在 Keychain 的做法相同）。

### tan 需要改的

| 步骤 | 内容 | 估计 |
|---|---|---|
| 配置 | `config.go` 的 `IOS` 加 `shareExtension: {types: ["text","url","image","file"], maxCount}` 与 `appGroup`（默认 `group.<id>`） | 小 |
| 源码 | `mobile/ios/share/ShareViewController.m`（约 200 行 ObjC，固定实现，不含用户代码） | 中 |
| 编译 | 第二次 clang 调用：`-fapplication-extension -e _NSExtensionMain`，链接 `Foundation UIKit UniformTypeIdentifiers` | 小 |
| 打包 | 生成扩展 Info.plist，放进 `PlugIns/`，`Assets.car` 可共用图标 | 小 |
| 签名 | 真机需要第二个描述文件（`ios.shareProvisioningProfile`），检查 App Group；先签 appex 再签 app；`-xcode` 工程生成器加第二个 target | 中-大（签名排错成本最高） |
| 运行时 | `internal/platform/ios`：启动/回到前台扫描 inbox、事件投递；`sys/share` 加 `OnReceive` | 中 |
| 验证 | 模拟器：Safari/照片 → 分享 → 选择应用 → 回到应用看事件 | 中 |

合计约 2–3 人日，其中真机签名（两个描述文件、App Group 能力在开发者后台开启）不确定性最大，所以本轮不做。

### iOS 小组件

WidgetKit 扩展的入口必须是 SwiftUI `Widget`（`@main struct ... : Widget`，`TimelineProvider`），没有 ObjC 入口；tan 目前不编译 Swift。需要：调用 `swiftc`（或生成 Xcode 目标）编译一个固定的 SwiftUI 小组件，读取 App Group 里主应用写的 JSON（`widget.Update` 在 iOS 上写共享容器 + `WidgetCenter.reloadAllTimelines()`——后者也只有 Swift API，需要一小段 Swift 或经由 `@objc` 包装）。工作量与分享扩展相当，外加 Swift 工具链与 Swift 运行时链接的处理，约 3 人日。

## 4. Android 分享目标（建议下一步，成本低）

在 `TangoActivity` 加 `ACTION_SEND` / `ACTION_SEND_MULTIPLE` 的 intent-filter（tan.json 开关，和 `urlSchemes` 一样由 `build_android_app.go` 插入），`Mobile.handleIntent` 读出 `EXTRA_TEXT` / `EXTRA_STREAM`（content URI 复制到缓存目录，同 `Dialogs` 的做法），作为事件投递。无需新进程、无签名问题，约 0.5 人日；事件负载与 iOS 一样需要契约里的共享内容字段，宜两端一起设计。
