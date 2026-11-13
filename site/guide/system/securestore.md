# securestore：在系统保护存储里保存小型机密

`securestore` 把令牌、密钥、密码等小型机密保存在平台提供的受保护存储里：iOS 和 macOS 用 Keychain（应用的通用密码项，仅本机、首次解锁后可用）；Android 用 Android Keystore 中的 AES 密钥加密后，存进应用私有的 SharedPreferences。

不要把它当数据库：值应当很小（KB 级）。

```go
import "github.com/tangoui/tango/sys/securestore"
```

## 用法

```go
if err := securestore.SetString(app, "session", token); err != nil {
	log.Println("save token:", err)
}

tok, err := securestore.GetString(app, "session")
if errors.Is(err, securestore.ErrNotFound) {
	// 从未保存过
}

_ = securestore.Delete(app, "session") // 不存在也不算错
```

二进制值：

```go
_ = securestore.Set(app, "db-key", key[:])
b, err := securestore.Get(app, "db-key")
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Set(a, key, []byte)` / `SetString(a, key, string)` | 保存，覆盖旧值 |
| `Get(a, key) ([]byte, error)` / `GetString(a, key) (string, error)` | 读取；不存在返回 `ErrNotFound` |
| `Delete(a, key)` | 删除；删除不存在的 key 不是错误 |
| `ErrNotFound` / `ErrUnsupported` | 未保存过 / 后端没有安全存储 |

完整签名见 [API 参考](/reference/api/sys-securestore)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| ✓ Keychain | — | — | ✓ Keychain | ✓ Keystore AES-GCM + 私有 SharedPreferences |

> 移动端能力仅在模拟器上验证，属实验性，尚未上真机。Windows、Linux 目前没有实现，返回 `ErrUnsupported`。

## 在页面里调用

目前没有 jsapi 对应。机密最好留在 Go 侧，只把需要的结果交给页面；确需暴露时用 `bind.Register`：

```go
type Account struct{ app *tango.App }

func (a Account) SignedIn() bool {
	_, err := securestore.GetString(a.app, "session")
	return err == nil
}

bind.Register(r, "Account", Account{app})
```

## 注意

- 可在任意 goroutine 调用，不需要主线程。
- 数据在应用更新后保留；iOS 上 Keychain 项在卸载应用后可能仍然存在。
- **iOS 模拟器**：没有任何 entitlement 的 ad-hoc 签名包访问 Keychain 会失败，`securestore` 报 `ErrNotConfigured`；需要把模拟器用的 entitlements（`application-identifier`、`keychain-access-groups`）链接进可执行文件后再签名。真机用描述文件签名即可。见 [移动端](/guide/mobile)。
- 不需要在 `tan.json` 的 `permissions` 里声明。
