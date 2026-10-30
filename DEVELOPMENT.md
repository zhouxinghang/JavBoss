# JavBoss 开发者说明

## 开发环境依赖

- Go `1.25.1` 或更高版本
- Node.js 24 和 npm（与 PR 检查和 Docker 构建一致）

## 技术栈

- Backend: Go + Gin + GORM + SQLite
- Frontend: React + Vite + Tailwind + Zustand
- 媒体探测: `ffprobe`
- 缩略图截图生成: macOS 使用 `ffmpeg`，其他平台使用 `mpv`
- 播放与手动截图: `mpv`

## 常用命令

下载依赖（`ffprobe` + `mpv`，macOS 额外下载 `ffmpeg`）：

```bash
./scripts/cli.sh download linux-x86_64
```

安装前端依赖：

```bash
cd web
npm install
```

启动后端：

```bash
./scripts/cli.sh dev backend
```

按 Docker 运行时配置启动本地后端（用于调试容器模式行为）：

```bash
DOCKER_MODE=1 ./scripts/cli.sh dev backend
```

启动前端：

```bash
./scripts/cli.sh dev frontend
```

前端检查：

```bash
cd web
npm test
npm run lint
npm run format:check
npm run build
```

打包发布：

```bash
scripts/cli.sh release linux-x86_64 v0.1.0
```

## 标题翻译 API

`PATCH /config` 支持 `deepseek_api_key`、`deepseek_model`（默认 `deepseek-flash`）和 `deepseek_auto_translate`（默认关闭）。省略密钥保留已保存值，传空字符串清除。配置响应只包含 `deepseek_api_key_configured` 状态，不返回密钥。

`POST /jav/title-translations` 接收 JSON `{"title":"原始标题"}`，原始标题须为 1–5000 字；成功返回 `{"zh_title":"中文标题"}` 草稿，不写入作品。缺少密钥或标题无效时返回 400，DeepSeek 请求失败、返回无效译文或未完整生成时返回 502。无查询参数。

`PUT /jav/items/:id` 可通过 `zh_title` 保存、修改或清空中文标题。`GET /jav` 的 `search` 同时匹配番号、原始标题和中文标题。自动翻译每分钟处理最多 20 条缺失中文标题的作品，仅在原始标题未变且中文标题仍为空时写入；遇到翻译错误结束本轮，下一轮重试。

## 通过 CLI 启停 Docker

需要 Node.js/npm、Docker 和 Docker Compose 插件。在仓库根目录执行：

```bash
# 构建镜像并后台启动容器
./scripts/cli.sh docker start

# 停止容器，保留容器和数据
./scripts/cli.sh docker stop
```

也可运行 `./scripts/cli.sh`，在第一层选择 `docker`，再选择 `start` 或 `stop`。

`start` 使用 `compose.local.yaml` 在 Docker 内构建前后端，默认镜像为 `javboss:local`，访问地址为 `http://localhost:5174`。数据保存在仓库根目录的 `docker-data/`，宿主机根目录只读挂载到 `/host`。重复启动会重新构建镜像并按需更新容器，继续使用该数据目录，不会自动迁移其他目录的数据。

更多参数可通过 `./scripts/cli.sh docker --help` 查看。

本地 Compose 构建默认使用 [DaoCloud 国内镜像站](https://github.com/DaoCloud/public-image-mirror)拉取 Node、Go、Alpine、distroless 和 Dockerfile 构建前端镜像，无需配置 VPN/HTTP 代理。npm 和 Go 依赖继续使用已有的国内源。

可通过 `JAVBOSS_DOCKER_HUB_MIRROR`、`JAVBOSS_DOCKER_GCR_MIRROR` 和 `JAVBOSS_DOCKERFILE_FRONTEND` 切换镜像站。例如恢复官方镜像源：

```bash
JAVBOSS_DOCKER_HUB_MIRROR=docker.io \
JAVBOSS_DOCKER_GCR_MIRROR=gcr.io \
JAVBOSS_DOCKERFILE_FRONTEND=docker/dockerfile:1.7 \
./scripts/cli.sh docker start --build-only
```

镜像标签及 distroless 的固定 SHA256 摘要保持不变。直接执行 `docker build` 或 GitHub Actions 发布仍默认使用官方镜像源；FFmpeg 二进制文件仍从 GitHub Releases 下载。

## 项目结构

```text
cmd/server             Go 服务入口
cmd/javprovider        JAV 元数据 provider 调试入口
internal/common        全局状态与共享配置
internal/db            GORM 模型查询与 SQLite 存储
internal/jav           JAV 元数据与女优资料抓取
internal/manager       封面下载与截图任务
internal/models        数据模型定义
internal/mpv           mpv 播放、快捷键与手动截图配置
internal/server        HTTP API 与静态资源路由
internal/service       目录扫描、JAV 识别、资料补全
internal/util          文件、系统、代理、视频探测等工具
web/                   React + Tailwind 前端
scripts/cli            开发、依赖下载与发布辅助 CLI
data/                  运行期数据库、封面、缩略图与缓存
```
