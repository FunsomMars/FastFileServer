# FileShare

> 高性能单根目录文件共享 HTTP 服务器 — Go 实现
>
> 替换 Python `http.server`，原生支持**并发下载**、**大文件零拷贝**、**断点续传**、**美观 Web UI**

---

## ✨ 特性

| 能力 | 说明 |
|---|---|
| 🚀 **并发下载** | 每连接独立 goroutine，数百客户端同时下载不阻塞 |
| 📦 **大文件零拷贝** | Linux `sendfile` / Windows `TransmitFile`，5GB 文件内存占用恒定 ~20MB |
| 🔁 **断点续传** | 完整支持 HTTP Range / 206 / ETag / If-Modified-Since |
| 📁 **递归浏览** | 单根目录，遍历所有子目录，HTML 美观界面 + 搜索 + 排序 + 暗色模式 |
| ⬆️ **上传** | 浏览器拖拽 / curl PUT / multipart POST 三种方式 |
| 🔐 **可选认证** | Basic Auth（`user:pass`） |
| 🛡️ **安全** | 路径遍历防御、符号链接默认拒绝、上传默认防覆盖 |
| 🪟🐧 **跨平台** | 单二进制，Windows / Linux 双向 amd64 + arm64，无需运行时 |
| ⚡ **响应迅速** | 浏览器 / API 响应亚毫秒级 |

---

## 📊 性能对比

| 维度 | Python `http.server` | FileShare (Go) |
|---|---|---|
| 并发 | ❌ 单线程串行 | ✅ 数百 goroutine |
| 大文件 | ❌ 内存随文件暴涨 | ✅ sendfile 零拷贝，O(常数) |
| Range / 断点续传 | ⚠️ 不完整 | ✅ 完整 |
| 20×1GB 并发下载 | ❌ 卡死 | ✅ ~28s，内存 +2.8MB |
| 部署 | 需 Python 环境 | 单一 7MB 二进制 |

---

## 🚀 快速开始

### 下载预编译二进制

从 `dist/` 目录获取对应平台的可执行文件：

| 文件 | 平台 |
|---|---|
| `fileserver-linux-amd64` | Linux x86_64 |
| `fileserver-linux-arm64` | Linux ARM64 (树莓派 / 鲲鹏 / Apple Silicon) |
| `fileserver-windows-amd64.exe` | Windows 10/11/Server x64 |
| `fileserver-windows-arm64.exe` | Windows ARM64 |

### 启动

#### Linux / macOS

```bash
chmod +x fileserver-linux-amd64
./fileserver-linux-amd64 -root /data/share -addr :8080
```

#### Windows (PowerShell / CMD)

```cmd
fileserver-windows-amd64.exe -root D:\share -addr :8080
```

#### 带上传 + Basic Auth

```bash
./fileserver-linux-amd64 \
  -root /data/share \
  -addr 0.0.0.0:8080 \
  -upload \
  -auth admin:yourPassword
```

打开浏览器访问 `http://<host>:8080` 即可看到 UI。

#### 后台模式（不阻塞命令行）

加 `-daemon` 启动后台运行：启动器会貃离终端、后台 fork 出独立会话进程，然后立即退出命令行，命令行可以继续输入下一条命令。日志处理与阻塞模式完全一致（只是默认输出改为 `-log` 指定的文件，不填为 stdout，但在被 fork 后会被重定向到 `/dev/null`）。

```bash
# 后台启动后不阻塞
./fileserver-linux-amd64 \
  -root /data/share \
  -addr :8080 \
  -daemon \
  -pidfile /var/run/fileserver.pid \
  -log /var/log/fileserver/access.log

# 查看状态
curl -s http://localhost:8080/api/info | head

# 优雅退出（发 SIGTERM 给 pid 文件中的进程）
kill "$(cat /var/run/fileserver.pid)"

# 手动轮转日志后发送 SIGHUP 重新打开日志文件
kill -HUP "$(cat /var/run/fileserver.pid)"
```

**后台模式行为约定**

- 启动器在 fork 后只打印 `pid / addr / root / upload / log / pidfile` 这几行基础信息，之后立即返回不阻塞 shell。
- 被 fork 出的守护进程在标准错误 / log 中只保留 `fileserver daemonized / pid / addr` 这一档最简 banner，与阻塞模式的详情 banner 区别开来。
- 日志处理**与阻塞模式完全相同**：访问日志仍走 `log.go` 中间件，信号仍走 graceful shutdown，区别仅在于启动器 vs 守护进程的「启动横幅」代涵不同。
- 同样启动两次会检测 `-pidfile` 冲突并以非零退出码拒绝。

---

## 📖 命令行参数

```
fileserver [flags]

Flags:
  -root string          服务根目录（必填，递归遍历所有文件）
  -addr string          监听地址 (default ":8080")
  -upload               启用文件上传（PUT/POST/DELETE）
  -max-upload int       单文件上传大小限制（字节），0 = 不限
  -overwrite            允许上传覆盖已存在文件（默认拒绝）
  -no-listing           关闭目录浏览（仅允许 /files/* 下载）
  -follow-symlinks      允许跟随符号链接（默认拒绝，防逃逸）
  -auth user:pass       启用 Basic Auth，例如 admin:123456
  -log string           访问日志文件路径（默认 stdout）
  -pretty               人类友好的大小显示（默认 true）
  -daemon               后台模式：脱离终端，立即打印精简信息后退出命令行
  -pidfile string       后台模式时将 PID 写入该文件（可选）
```

---

## 🌐 HTTP 接口

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/` | 浏览首页（HTML） |
| `GET` | `/{path...}` | 浏览子目录（HTML） |
| `GET` | `/api/list?path=/foo` | JSON 列表 API |
| `GET` | `/api/info` | 服务器信息 |
| `GET` | `/files/*` | 下载文件（支持 Range / 206 / ETag / HEAD） |
| `PUT` | `/files/*` | 上传文件（需 `-upload`） |
| `POST` | `/upload?path=/dest` | Multipart 多文件上传（需 `-upload`） |
| `DELETE` | `/files/*` | 删除文件（需 `-upload`） |
| `GET` | `/static/*` | 静态资源（CSS / JS） |

### 示例

#### 下载（支持断点续传）

```bash
# 普通下载
curl -O http://host:8080/files/movie.mp4

# 断点续传
curl -C - -O http://host:8080/files/movie.mp4

# 指定分片（多线程下载工具如 aria2 / axel 自动使用）
curl -H "Range: bytes=0-1048575" -o part1.bin http://host:8080/files/movie.mp4
```

#### 上传

```bash
# 浏览器：拖拽到上传弹窗

# curl PUT（单文件）
curl -X PUT --data-binary "@local.txt" http://host:8080/files/remote.txt

# curl multipart（多文件）
curl -X POST \
  -F "files=@a.txt" \
  -F "files=@b.png" \
  "http://host:8080/upload?path=/some-dir"
```

#### 认证

```bash
curl -u admin:secret123 http://host:8080/
```

---

## 🛡️ 安全特性

- **路径遍历防御**：`..`、`%2e%2e`、`%2f` 等均被拒绝（实测 HTTP 400）
- **符号链接默认拒绝**：避免软链逃逸出 root；可用 `-follow-symlinks` 开启
- **上传防覆盖**：默认拒绝覆盖已存在文件，需 `-overwrite` 显式开启
- **Basic Auth**：可选的 `user:pass` 认证
- **安全响应头**：`X-Content-Type-Options: nosniff`、`Referrer-Policy: no-referrer`、`X-Frame-Options: SAMEORIGIN`
- **Slowloris 防护**：`ReadHeaderTimeout = 10s`
- **大文件不超时**：`ReadTimeout=0, WriteTimeout=0`，避免长时间下载被切断

---

## 🪟 部署指南

### Linux (systemd 服务)

创建 `/etc/systemd/system/fileshare.service`：

```ini
[Unit]
Description=FileShare HTTP Server
After=network.target

[Service]
Type=simple
User=fileshare
Group=fileshare
WorkingDirectory=/opt/fileshare
ExecStart=/opt/fileshare/fileserver-linux-amd64 \
  -root /data/share \
  -addr 0.0.0.0:8080 \
  -upload \
  -auth admin:ChangeMe123 \
  -log /var/log/fileshare/access.log
Restart=on-failure
RestartSec=5
LimitNOFILE=65535

# 安全加固
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/data/share /var/log/fileshare

[Install]
WantedBy=multi-user.target
```

启用：

```bash
sudo useradd -r -s /sbin/nologin fileshare
sudo mkdir -p /opt/fileshare /var/log/fileshare
sudo cp fileserver-linux-amd64 /opt/fileshare/
sudo chown -R fileshare:fileshare /opt/fileshare /var/log/fileshare
sudo systemctl daemon-reload
sudo systemctl enable --now fileshare
sudo systemctl status fileshare
```

查看日志：`journalctl -u fileshare -f`

### Windows

#### 方式 1：直接运行（适合临时 / 测试）

```cmd
fileserver-windows-amd64.exe -root D:\share -addr :8080 -upload
```

#### 方式 2：注册为 Windows 服务（使用 NSSM）

1. 下载 [NSSM](https://nssm.cc/) 解压到 `C:\nssm\`
2. 执行：

```cmd
C:\nssm\win64\nssm.exe install FileShare "C:\share\fileserver-windows-amd64.exe" "-root D:\share -addr :8080 -upload"
C:\nssm\win64\nssm.exe set FileShare AppDirectory C:\share
C:\nssm\win64\nssm.exe set FileShare AppStdout C:\share\logs\access.log
C:\nssm\win64\nssm.exe set FileShare AppStderr C:\share\logs\error.log
C:\nssm\win64\nssm.exe set FileShare AppRotateFiles 1
C:\nssm\win64\nssm.exe set FileShare AppRotateBytes 10485760
C:\nssm\win64\nssm.exe start FileShare
```

#### 方式 3：Windows 任务计划程序（开机自启）

```powershell
$Action = New-ScheduledTaskAction `
  -Execute "C:\share\fileserver-windows-amd64.exe" `
  -Argument "-root D:\share -addr :8080 -upload"
$Trigger = New-ScheduledTaskTrigger -AtLogOn
Register-ScheduledTask -TaskName "FileShare" `
  -Action $Action -Trigger $Trigger `
  -User "SYSTEM" -RunLevel Highest
```

### Docker（可选）

```dockerfile
FROM scratch
COPY fileserver-linux-amd64 /fileserver
EXPOSE 8080
ENTRYPOINT ["/fileserver"]
```

```bash
docker build -t fileshare .
docker run -d --restart=unless-stopped \
  -p 8080:8080 \
  -v /data/share:/share:ro \
  -v /data/inbox:/inbox \
  fileshare \
  -root /share -addr :8080 -upload
```

### 防火墙提示

仅监听本地：
```bash
-addr 127.0.0.1:8080
```

监听所有网卡：
```bash
-addr 0.0.0.0:8080
```

允许端口通过防火墙（Ubuntu）：
```bash
sudo ufw allow 8080/tcp
```

---

## 🏗️ 从源码编译

需要 Go 1.22+：

```bash
git clone <repo>
cd httpServer
./build.sh     # 产出 dist/ 下的 4 个平台二进制
# 或单平台构建
go build -trimpath -ldflags="-s -w" -o fileserver .
```

---

## 🧪 自验证清单

启动后可执行下列检查：

```bash
# 1. 首页
curl -I http://localhost:8080/

# 2. 列表 JSON API
curl http://localhost:8080/api/list

# 3. 下载
curl -O http://localhost:8080/files/<some-file>

# 4. Range（断点续传）
curl -H "Range: bytes=0-1023" -o /tmp/p1 http://localhost:8080/files/<some-file>
curl -H "Range: bytes=1024-2047" -o /tmp/p2 http://localhost:8080/files/<some-file>
cat /tmp/p1 /tmp/p2 > /tmp/joined
diff /tmp/joined <(dd if=<some-file> bs=1 count=2048 2>/dev/null) && echo "Range OK"

# 5. 并发下载（验证内存恒定）
dd if=/dev/zero of=/tmp/share/1G.bin bs=1M count=1024
for i in $(seq 1 20); do
  curl -s -o /dev/null http://localhost:8080/files/1G.bin &
done
wait
ps -p $(pgrep fileserver) -o rss=  # 应 < 30MB
```

---

## 📂 项目结构

```
httpServer/
├── main.go              # 入口、flag 解析、路由注册
├── server.go            # 下载 handler、路径安全校验、安全头
├── browse.go            # 目录扫描、HTML 模板渲染、JSON API
├── upload.go            # PUT 和 multipart 上传、DELETE
├── auth.go              # Basic Auth 中间件
├── log.go               # 访问日志中间件
├── embed_assets.go      # //go:embed web/* 打包静态资源
├── web/
│   ├── index.html       # 浏览页模板
│   ├── style.css        # 样式（暗色自适应）
│   └── app.js           # 搜索/排序/上传/快捷键
├── go.mod
├── build.sh             # 交叉编译脚本
├── dist/                # 编译产物
│   ├── fileserver-linux-amd64
│   ├── fileserver-linux-arm64
│   ├── fileserver-windows-amd64.exe
│   └── fileserver-windows-arm64.exe
└── README.md
```

---

## ⌨️ 浏览器快捷键

| 键 | 功能 |
|---|---|
| `/` | 聚焦搜索框 |
| `Esc` | 清空搜索 / 关闭弹窗 |
| `R` | 刷新列表 |

---

## 🔧 调优建议

| 场景 | 建议 |
|---|---|
| 文件极大（>100GB）| 保持默认设置即可，sendfile 已最优 |
| 客户端网络很慢 | 保持 `WriteTimeout=0`，避免大文件传输中断 |
| 高并发（数千）| 增加系统文件描述符上限：`ulimit -n 65535` |
| 磁盘慢 | 不影响 server，但下载带宽受限于磁盘读速度 |
| 内网高速环境 | 默认 `Accept-Ranges: bytes` 允许多客户端分片并发拉同一文件 |

---

## 📜 License

MIT