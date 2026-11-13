# 移动端权限声明（Info.plist / AndroidManifest）

本文件列出 `sys/...` 能力包在 iOS 和 Android 上需要的声明。`tan build` 根据 `tan.json` 的 `"permissions"` 生成它们，名字就是下表第一列（与 `sys/permission` 的 `Kind` 一致）：

```json
"permissions": ["camera", "photos", "location", "biometric", "notifications"],
"ios": { "usageDescriptions": { "camera": "扫描二维码时使用相机。" } }
```

iOS 上缺用途说明键时直接调用相关 API 会让系统终止应用；Tango 在调用前检查，缺键时返回 `ErrNotConfigured`（错误文本写出缺的键名）而不是崩溃。Android 上清单没声明的运行时权限，`permission.Request` 返回 `ErrNotConfigured`。

## 按权限

| 名字 | 用到它的 API | iOS Info.plist 键 | AndroidManifest |
|---|---|---|---|
| `camera` | `media.TakePhoto`；`permission`（camera） | `NSCameraUsageDescription` | `android.permission.CAMERA`；`<uses-feature android:name="android.hardware.camera" android:required="false"/>`。拍照走系统相机，不声明也能用；**一旦声明**，拍照前必须先获授权（Tango 会先请求） |
| `photos` | `permission`（photos）。`media.Pick` 用系统选择器，**不需要**此权限 | `NSPhotoLibraryUsageDescription`（读）、`NSPhotoLibraryAddUsageDescription`（存入相册时） | `READ_MEDIA_IMAGES`、`READ_MEDIA_VIDEO`（33+）、`READ_MEDIA_VISUAL_USER_SELECTED`（34+，部分授权 → `limited`）、`READ_EXTERNAL_STORAGE`（`android:maxSdkVersion="32"`） |
| `location` | `location.Current` / `Watch`；`permission`（location） | `NSLocationWhenInUseUsageDescription` | `ACCESS_FINE_LOCATION`、`ACCESS_COARSE_LOCATION`；`<uses-feature android:name="android.hardware.location" android:required="false"/>` |
| `location-always` | `permission`（location-always） | 在 `location` 之外再加 `NSLocationAlwaysAndWhenInUseUsageDescription`；后台持续定位还需 `UIBackgroundModes` 含 `location` | 在 `location` 之外再加 `ACCESS_BACKGROUND_LOCATION`（29+；30+ 系统只允许在设置里授予） |
| `microphone` | `permission`（microphone） | `NSMicrophoneUsageDescription` | `RECORD_AUDIO` |
| `biometric` | `biometric.Authenticate` | `NSFaceIDUsageDescription`（Touch ID 机型不需要，但声明无害） | `USE_BIOMETRIC`（28+）、`USE_FINGERPRINT`（`android:maxSdkVersion="27"`） |
| `notifications` | `permission`（notifications）、`push.Register`、`notify` | 无键（运行时询问）。远程推送另需 entitlement `aps-environment`（描述文件开启 Push Notifications）；后台静默推送需 `UIBackgroundModes` 含 `remote-notification` | `POST_NOTIFICATIONS`（33+） |

不需要声明的：`share`、`haptics`（Android `performHapticFeedback` 不需要 `VIBRATE`）、`safearea`、`orientation`、`securestore`（iOS Keychain、Android Keystore）。

`network`、`power`、`shell` 不需要在 `tan.json` 里声明：Android 的 `ACCESS_NETWORK_STATE`（`sys/network`，普通权限）已在宿主清单模板里；`power.KeepAwake` 想在息屏时保持 CPU 运行需要应用自己在清单加 `android.permission.WAKE_LOCK`（普通权限，不声明时退化为保持屏幕常亮）。`notify` 在 iOS 上无需任何键，第一次发送时系统询问；Android 13+ 必须声明 `notifications`，否则 `notify.Send` 返回 `ErrNotConfigured`。

## 其他声明

- **分享文件 / 拍照（Android）**：宿主自带的 `org.tango.host.TangoFileProvider`（`android:authorities="${applicationId}.tango.files"`、`exported="false"`、`grantUriPermissions="true"`）已在 `mobile/android` 的清单里，生成清单时保留。
- **方向锁定（iOS）**：只能在 `UISupportedInterfaceOrientations` 声明的方向里选；iPad 支持分屏多任务时系统忽略锁定（需要锁定就加 `UIRequiresFullScreen`）。
- **Keychain（iOS 模拟器）**：ad-hoc 签名、没有任何 entitlement 的模拟器包调用 Keychain 返回 `errSecMissingEntitlement`（`securestore` 报 `ErrNotConfigured`）。Xcode 的做法是把模拟器用的 entitlements（`application-identifier`、`keychain-access-groups`）作为 `__TEXT,__entitlements` 段链接进可执行文件（`-Wl,-sectcreate,__TEXT,__entitlements,<plist>`），再 ad-hoc 签名；已验证这样 `securestore` 在模拟器上读写正常。真机用描述文件签名即可。
- **通用链接（iOS）**：entitlement `com.apple.developer.associated-domains` = `applinks:example.com`，网站放 `/.well-known/apple-app-site-association`。打开后走 `EventOpenURL`（冷启动、热启动都处理）。自定义 scheme 用 `tan.json` 的 `urlSchemes`（`CFBundleURLTypes`）。
- **App Links（Android）**：给 `TangoActivity` 加
  ```xml
  <intent-filter android:autoVerify="true">
      <action android:name="android.intent.action.VIEW" />
      <category android:name="android.intent.category.DEFAULT" />
      <category android:name="android.intent.category.BROWSABLE" />
      <data android:scheme="https" android:host="example.com" />
  </intent-filter>
  ```
  网站放 `/.well-known/assetlinks.json`。`onCreate` 与 `onNewIntent` 的 `ACTION_VIEW` 都走 `EventOpenURL`。
- **隐私清单（iOS）**：`mobile/ios/PrivacyInfo.xcprivacy` 是模板，列出 Tango 自身用到的 required reason API（文件时间戳 C617.1/3B52.1、系统启动时间 35F9.1），复制进 .app 根目录；应用自己用到的再补充。

## Android 推送（FCM）接入

Tango 不打包 Google 服务，`push.Register` 在 Android 上默认返回 `ErrUnsupported`。接入步骤：

1. 在 `mobile/android` 的 Gradle 工程里按 Firebase 文档加 `google-services` 插件、`google-services.json` 和 `com.google.firebase:firebase-messaging` 依赖，清单声明 `POST_NOTIFICATIONS`（`notifications`）。
2. 写一个 `FirebaseMessagingService` 子类并在清单注册，在回调里转给 Tango：
   ```java
   public class MyFcm extends FirebaseMessagingService {
       @Override public void onNewToken(String token) { org.tango.host.TangoPush.token(token); }
       @Override public void onMessageReceived(RemoteMessage m) {
           RemoteMessage.Notification n = m.getNotification();
           org.tango.host.TangoPush.received(m.getData(), n != null ? n.getTitle() : null, n != null ? n.getBody() : null);
       }
   }
   ```
   应用启动时（或 `Register` 前）调用一次 `FirebaseMessaging.getInstance().getToken().addOnSuccessListener(TangoPush::token)`。
   自己处理通知点击时可调用 `TangoPush.opened(data, title, body, action)`。
3. 之后 `push.Register` 返回 FCM 令牌，`push.OnToken` / `OnReceive` 生效；点击通知启动应用（Intent 带 `google.message_id`）时宿主自动发 `OnOpen`。

`TangoPush` 的确切方法签名见 `mobile/android/app/src/main/java/org/tango/host/TangoPush.java`。
