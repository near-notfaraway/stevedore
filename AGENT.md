# Stevedore Agent 指南

## 项目概述

Stevedore 是一个使用 Go 编写、支持 Linux 和 macOS 的四层 UDP 负载均衡器。它根据
数据包内容进行路由，然后通过加权轮询（`rr`）或加权一致性哈希（`chash`）
选择上游节点。Linux 是生产性能目标，使用 `epoll`、`recvmmsg` 和
`sendmmsg`；macOS 使用 `kqueue` 和基于 `recvfrom`/`sendmsg` 的兼容路径。

- 模块：`github.com/near-notfaraway/stevedore`
- Go 版本：以 `go.mod` 为准
- 程序入口：`main.go`
- 配置示例：`sd_config/config.example.json`
- 支持的配置格式：JSON/JSONC 和 YAML
- 运行目标：Linux amd64、Darwin amd64/arm64
- 命令行配置参数：`-config`

## 请求流程

1. `main.go` 加载配置、按需启动 pprof、初始化日志并启动
   `sd_server.Server`。
2. Linux 上，`sd_server.Server` 创建 `ListenParallel` 个独立 Reactor。macOS
   的 `SO_REUSEPORT` 不提供 Linux 式多 listener 分流，因此有效并行度固定为
   1，配置值大于 1 时记录警告。每个 Reactor
   独占一个启用 `SO_REUSEADDR`/`SO_REUSEPORT` 的监听套接字、一个边缘触发
   selector、一张 Session 表、一个共享 RX Container 和一个下行 TX
   Container。
3. Reactor 直接通过平台批量接收接口读取客户端数据包，不经过任务池、
   Channel 或额外 Worker。每个 fd 每轮最多执行 `MaxRecvBatchesPerFD` 次
   批量接收；未排空的 fd 进入 Reactor 本地 ready queue，并通过 selector
   自唤醒下一轮，避免热点 fd 饿死其他 Session。Linux 自唤醒使用 `eventfd`，
   macOS 使用 `EVFILT_USER`。
4. Linux 下游地址由内核 `SO_REUSEPORT` 哈希稳定分配到 Reactor；macOS
   仅运行单 Reactor。Reactor 本地 Session 持有用于上游通信的 UDP fd，并
   注册到同一个 selector。
5. `sd_upstream.Registry` 共享健康检查、版本化 Peer 快照和只读
   `sd_route.Router`；每个 Reactor 拥有独立 `Manager`、RR 游标、RR 列表
   和一致性哈希表。Router 按配置顺序检查路由，首个匹配项生效；没有匹配项
   时使用 `DefaultUpstream`。空字符串和保留值 `drop` 表示丢弃，其他
   Upstream 引用在启动时校验。
6. 选中的 Upstream 通过加权轮询或一致性哈希选择健康 Peer，并发送数据包。
7. 同一个 Reactor 批量读取 Session fd 的上游响应，通过自己的监听 fd
   发送回原始下游地址。Linux 的 TX iovec 直接引用当前 RX batch payload，
   使用 `sendmmsg` 批量发送；macOS 通过同一 Container 接口逐包调用
   `sendmsg`。Session 活跃时在 Reactor 本地侵入式时间轮中 O(1) 续期；
   selector 超时周期只处理当前到期桶，不遍历完整 Session 表。

## 包职责

- `sd_config`：仅定义配置结构。
- `sd_server`：服务生命周期、分片 Reactor、Reactor 本地 Session 表及
  上传/下载数据包循环。
- `sd_session`：由 Reactor 独占的 Session 状态和侵入式过期时间轮。
- `sd_socket`：平台 selector、套接字、sockaddr 和批量消息接口封装。带
  `_linux.go`/`_darwin.go` 后缀的文件持有平台系统调用细节。
- `sd_route`：不可变 payload 路由规则、顺序匹配和默认 Upstream 选择。
- `sd_upstream`：共享 Peer/健康检查控制面，以及 Reactor 本地轮询和一致性
  哈希数据面。
- `sd_util`：配置解码、字节匹配和日志。
- `doc/media`：README 使用的图片。

保持以上包职责，不要将系统调用细节移入 Server 或业务路由代码。

## 开发规则

- 采用满足需求的最小改动，不做无关清理。
- 遵循现有命名和包边界。语义相同的上传/下载、RR/CHash 逻辑应尽量保持
  结构一致。
- 优先使用扁平控制流：校验、调用、错误处理、返回或响应。
- 保持路由顺序，因为首个匹配项生效属于既有行为契约。
- Router 在 Reactor 间共享；匹配状态必须只读。仅当
  `Log.RouteStatsIntervalSec > 0` 时启用并发安全指标计数。
- 路由匹配热路径必须保持零堆分配；Upstream 名称必须唯一。
- 将数据包切片视为有边界的输入，切片前必须检查长度。
- Listener、Session fd、selector 注册、Reactor 本地映射和关闭必须由所属
  Reactor 串行维护，不要重新引入跨 Reactor 的 Session 共享。
- Session 创建、活跃刷新和删除必须同步维护 Reactor 本地时间轮，禁止使用
  周期性全表扫描替代。
- ET fd 达到批次预算后必须重新排队并主动唤醒 Reactor，不能依赖内核再次
  产生可读边缘。
- 下行 TX 描述符仅能在对应 RX batch payload 有效期间使用；必须在下一次
  批量接收覆盖缓冲前完成发送。
- 不要绕过 `sd_socket.Selector` 或批量消息接口在 Reactor 中直接引用
  `unix.EpollEvent`、`unix.Kevent_t`、`MMsgHdr` 等平台结构。
- 维护共享 Peer 状态和查找结构的并发安全。涉及竞态风险的改动应在支持的
  本机平台运行 `go test -race ./...`。
- Peer 健康快照由 Registry 原子发布；RR 游标、RR 列表和一致性哈希表必须
  保持 Reactor 本地所有权。
- 不要仅为了通过测试而改变正确的生产逻辑。当实现契约本身正确时，应修正
  错误的断言或 Mock。
- 修改纯逻辑时添加聚焦的测试。网络生命周期改动应增加对应平台的集成测试，
  并通过另一平台的交叉构建。
- 所有改动过的 Go 文件都必须运行 `gofmt`。标准库或现有依赖足以解决问题
  时，不要引入新依赖。
- 新增或重命名配置字段时，同步更新 `sd_config/config.go`、
  `sd_config/config.example.json` 和 README 中的相关示例，并确保非目标
  环境不受影响。

## 构建与测试

在 macOS 中：

```bash
go test ./...
go test -race ./...
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet ./...
make build-darwin
```

在 Linux 环境中：

```bash
go test ./...
go test -race ./...
make build
```

常用 Make 目标：

```bash
make test
make test-race
make build
make build-darwin DARWIN_ARCH=arm64
```

`make build` 固定生成 Linux amd64 生产二进制。`make build-darwin` 默认
使用当前 Go 工具链的架构，也可通过 `DARWIN_ARCH=amd64` 或 `arm64` 指定。

## 打包与运行

`make build` 会在仓库根目录交叉编译名为 `stevedore` 的 Linux amd64
静态二进制文件。`make package` 会将其移动到
`output/stevedore/bin/`，将示例配置复制到 `output/stevedore/etc/`，
并生成 `output/stevedore.tgz`。

运行打包后的程序：

```bash
cd output/stevedore
bin/stevedore -config etc/config.example.json
```

示例日志路径相对于进程工作目录解析。健康检查和配置的 Peer 需要可访问的
UDP 端点，不要期待使用示例中的 `10.0.0.x` 地址完成成功的冒烟测试。

## Review 检查项

- 路由或哈希键切片前已检查数据包边界。
- 创建的所有 fd 都会在失败和关闭路径中释放。
- Session fd 仅由所属 Reactor 注册、访问和移除。
- Session 活跃时间和过期语义保持正确。
- Peer 列表更新不会与 Peer 选择发生竞态。
- 空 Peer 集合、非法权重和缺少备用 Peer 的情况得到安全处理。
- 路由和默认上游的数据包丢弃行为符合预期。
- Darwin 上不会为 `ListenParallel > 1` 创建冗余 Reactor。
- 使用上述命令通过本机测试、race 测试和 Linux 交叉构建。
