# biometric：用 Face ID / Touch ID / 指纹验证机主

`biometric` 弹出系统的生物识别验证框（iOS、macOS 上是 Face ID / Touch ID，Android 9+ 上是 `BiometricPrompt`），返回 nil 表示验证通过。常用于解锁本地加密的数据、确认敏感操作。

它只回答「是不是机主」，不提供密钥。需要和机密存储配合时，可以先验证再读 [securestore](/guide/system/securestore)。

```go
import "github.com/tangoui/tango/sys/biometric"
```

## 用法

```go
go func() { // Android 上必须在 goroutine 里调用
	kind, err := biometric.Available(app)
	if kind == biometric.None {
		log.Println("no biometry:", err) // ErrNotAvailable / ErrNotEnrolled / ErrLockedOut
		return
	}
	err = biometric.Authenticate(app, biometric.Options{
		Reason:        "解锁笔记",
		AllowPasscode: true,
	})
	switch {
	case err == nil:
		unlock()
	case errors.Is(err, biometric.ErrCanceled):
		// 用户取消
	case errors.Is(err, biometric.ErrFailed):
		// 验证失败
	}
}()
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Available(a) (Kind, error)` | 设备提供哪种生物识别；为 `None` 时 err 说明原因 |
| `Authenticate(a, Options) error` | 弹出系统验证框并等待结果 |
| `Options` | `Reason`（iOS/macOS 必填）、`Title`（Android 标题，默认应用名）、`CancelTitle`、`AllowPasscode`（允许退回设备密码 / PIN） |
| `Kind` 常量 | `None`、`TouchID`（含 Android 指纹）、`FaceID`（含 Android 人脸）、`OpticID`、`Other`（Android 上可用但不报告类型） |
| `String(k)` | `"none"`、`"touchID"`、`"faceID"`、`"opticID"`、`"biometric"` |
| 错误 | `ErrCanceled`、`ErrFailed`、`ErrNotAvailable`、`ErrNotEnrolled`、`ErrLockedOut`、`ErrNotConfigured`、`ErrUnsupported` |

完整签名见 [API 参考](/reference/api/sys-biometric)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| ✓ LocalAuthentication | — | — | ✓ `LAContext` | ✓ `BiometricPrompt`（API 28+） |

> 移动端能力仅在模拟器上验证，属实验性，尚未上真机。

## 在页面里调用

目前没有 jsapi 对应，需要时用 `bind.Register` 暴露自己的方法：

```go
type Lock struct{ app *tango.App }

func (l Lock) Unlock() error {
	return biometric.Authenticate(l.app, biometric.Options{Reason: "解锁笔记"})
}

bind.Register(r, "Lock", Lock{app})
```

bind 的方法在 goroutine 上执行，满足 Android 的要求。页面里 `await tango.call("Lock.Unlock")`。

## 注意

- **权限声明**：在 `tan.json` 的 `permissions` 里加 `"biometric"`。iOS 生成 `NSFaceIDUsageDescription`，缺少时 `Authenticate` 返回 `ErrNotConfigured` 而不是让系统终止应用；Android 生成 `USE_BIOMETRIC`（28+）和 `USE_FINGERPRINT`（27 及以下）。见 [tan.json](/reference/tan-json) 与 [移动端](/guide/mobile)。
- **线程**：调用会阻塞到用户操作完成。Android 上不要在主线程回调里调用，主线程上会直接返回 `tango.ErrNestedWait`。
- 后端没有生物识别时返回 `ErrUnsupported`；应用退出后返回 `tango.ErrNotRunning`。
