# 资源服务

页面的 HTML、JS、CSS、图片由 `Options.Assets` 提供，它就是一个普通的 `http.Handler`。后端把资源 origin（macOS / Linux 是 `tango://app`，Windows 是 `https://tango.localhost`）下的请求交给它，不开任何本地端口。

## 静态文件：asset.FS

```go
//go:embed all:frontend/dist
var dist embed.FS

site, _ := fs.Sub(dist, "frontend/dist")
app := tango.New(tango.Options{
    Assets: asset.FS(site, asset.Options{
        SPA: true, // 没有扩展名的未知路径回落到 index.html（前端路由）
        Headers: http.Header{"Content-Security-Policy": {"default-src 'self'"}},
    }),
})
```

`asset.Options`：`Index`（目录默认文件，默认 `index.html`）、`SPA`、`Headers`（加在每个成功响应上，适合放 CSP）。

## 开发时代理：asset.FromEnv

```go
Assets: asset.FromEnv(site, asset.Options{SPA: true})
```

`FromEnv` 在开发构建（`-tags tango_dev`，`tan dev` 自动加）里、环境变量 `TANGO_DEV_URL` 有值时返回指向该地址的反向代理（`asset.DevProxy`），否则返回 `asset.FS`。`tan dev` 启动前端开发服务器后会设置这个变量，所以同一行代码在开发时走 Vite（含 HMR），发布时走嵌入的文件。

默认构建（普通 `go build`/`go run .`、`tan build`）**不读** `TANGO_DEV_URL`，始终服务嵌入的文件：否则同用户的其他进程设置这个变量就能让已签名的应用加载任意页面。不用 `tan dev` 时手动调试：`TANGO_DEV_URL=http://localhost:5173 go run -tags tango_dev .`。

`DevProxy` 只接受回环目标（`localhost`、`127.0.0.0/8`、`::1`；Android `adb reverse` 和 iOS 模拟器都满足）。要代理到局域网里的 dev server，自己调用 `asset.DevProxyWith(url, asset.DevProxyOptions{AllowRemote: true})`。

资源源上 GET/HEAD 以外的请求，`Origin` 不是资源源本身时核心直接回 403，不会到达你的 Handler（防外站表单 POST 到 `/api`）；需要跨源写请求时设置 `tango.Options.AllowCrossOriginAssets`。

## 自己写 Handler

因为是 `http.Handler`，可以自由组合：

```go
mux := http.NewServeMux()
mux.Handle("/", asset.FS(site, asset.Options{}))
mux.HandleFunc("/api/thumb", serveThumbnail) // 动态生成的内容
app := tango.New(tango.Options{Assets: mux})
```

注意 `ServeHTTP` 可能在主线程之外的任意线程上被调用，Handler 要并发安全，也不要在里面等主线程。

## 展示用户文件：service/fileserver

要在页面里显示用户目录下的文件（下载、生成的图片），用 `service/fileserver` 把一个目录挂在某个前缀下：

```go
srv, _ := fileserver.New(downloadsDir, fileserver.Options{})
app := tango.New(tango.Options{
    Assets: fileserver.Mount("/files/", srv, asset.FS(site, asset.Options{})),
})
// 页面里 <img src="/files/report.png">
```

请求不能离开该目录：含 `..` 的路径被拒绝，文件通过 `os.Root` 打开，指向目录外的符号链接也不会被跟随；默认不列目录、不提供点文件。

## 加载外部 URL

`SurfaceOptions.URL` 可以是绝对地址（`https://example.com/`）。只有资源 origin 和核心最后一次导航到的 origin 发来的消息会被接受，见 [安全模型](/guide/concepts/security)。
