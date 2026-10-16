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
- 支持的配置格式：JSON/JSONC 和 `.yaml`
- 运行目标：Linux amd64、Darwin amd64/arm64
- 常用命令行参数：`-c`（配置）、`-t`（校验）、`-s`（信号）、`-v`（版本）
- 可热更新配置：仅 `Upload` 段

## 请求流程

1. `main.go` 解析互斥的运行、校验、信号和版本动作。运行与校验动作先通过
   `sd_config.Load` 加载配置并填充 `Common` 默认路径，再依次执行
   `sd_config.ValidateConfig` 和 `sd_server.ValidateRuntimeConfig`。运行时切换
   工作目录、初始化日志和 PID 文件，按需启动 pprof，最后启动
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
   仅运行单 Reactor。Reactor 本地 Session 按 IPv4、IPv6 地址族分别持有
   上游 UDP fd，并注册到同一个 selector。
5. `sd_server.uploadRegistry` 共享只读路由规则、健康检查和版本化 Peer
   快照；每个 Reactor 拥有独立 Router、Upstream selector、RR 游标、RR
   列表、一致性哈希表和统计计数器。Server 汇总各 Reactor 的统计快照。
   Router 按配置顺序检查路由，首个匹配项生效；没有匹配项时使用
   `DefaultUpstream`。空字符串和保留值 `drop` 表示丢弃，其他 Upstream
   引用在启动时校验。
6. 选中的 Upstream 通过加权轮询或一致性哈希选择健康 Peer，并发送数据包。
   CHash key 支持固定区间、多区间拼接、掩码和长度字段提取；无效 key 按
   `InvalidKeyPolicy` 丢弃或回退 RR。每个 Upstream 最多有一个 Backup
   Peer，它平时正常参与负载均衡，仅在所有 Peer 不可用时强制兜底。
7. 同一个 Reactor 批量读取 Session fd 的上游响应，通过自己的监听 fd
   发送回原始下游地址。Linux 的 TX iovec 直接引用当前 RX batch payload，
   使用 `sendmmsg` 批量发送；macOS 通过同一 Container 接口逐包调用
   `sendmsg`。Session 活跃时在 Reactor 本地侵入式时间轮中 O(1) 续期；
   selector 超时周期只处理当前到期桶，不遍历完整 Session 表。
8. 收到 `SIGHUP` 或 `-s reload` 时，主进程重新读取原配置文件，只允许
   `Upload` 发生变化。Server 先完整构建新的 Registry 和各 Reactor 的
   Manager，再通过 Reactor 命令切换；部分切换失败时尝试回滚。切换成功后
   关闭旧健康检查，已有 Session 保持不变。`SIGTERM`、`SIGQUIT` 和
   `SIGINT` 会唤醒 selector，并等待所有 Reactor 释放资源后退出。

## 包职责

- `sd_config`：命令行选项、配置结构、文件加载、`Common` 默认值填充、基础
  字段校验和非 `Upload` 热更新差异校验。
- `sd_diagnosis`：日志初始化，以及路由和 Upstream 统计的周期输出。
- `sd_server`：服务生命周期、分片 Reactor、Reactor 本地 Session 表及
  上传/下载数据包循环，并负责 `Upload` 配置的构建、切换和回滚。
- `sd_session`：由 Reactor 独占的 Session 状态和侵入式过期时间轮。
- `sd_socket`：平台 selector、套接字、sockaddr 和批量消息接口封装。带
  `_linux.go`/`_darwin.go` 后缀的文件持有平台系统调用细节。
- `sd_route`：不可变 payload 路由规则、顺序匹配和默认 Upstream 选择。
- `sd_upstream`：共享 Peer/健康检查控制面，以及 Reactor 本地轮询和一致性
  哈希数据面。
- `sd_util`：配置文件解码、字节转换与匹配，以及 PID 文件管理。

保持以上包职责，不要将系统调用细节移入 Server 或业务路由代码。

## 开发规则

- 采用满足需求的最小改动，不做无关清理。
- 遵循现有命名和包边界。语义相同的上传/下载、RR/CHash 逻辑应尽量保持
  结构一致。
- 优先使用扁平控制流：校验、调用、错误处理、返回或响应。
- 保持路由顺序，因为首个匹配项生效属于既有行为契约。
- Router 和 Upstream 统计状态归 Reactor 本地所有，Server 通过原子快照
  聚合；仅当 `Log.UploadStatsIntervalSec > 0` 时启用指标计数。
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
- 热更新必须先完整构建并校验候选 `Upload`，再通知 Reactor 切换。不得在
  构建失败时修改当前代，也不得因热更新清理已有 Session。
- `ValidateUploadReload` 只比较并拒绝非 `Upload` 配置变化，不填充默认值；
  配置归一化统一由 `Load` 调用 `applyCommonDefaults` 完成。
- 不要仅为了通过测试而改变正确的生产逻辑。当实现契约本身正确时，应修正
  错误的断言或 Mock。
- 修改纯逻辑时添加聚焦的测试。网络生命周期改动应增加对应平台的集成测试，
  并通过另一平台的交叉构建。
- 所有改动过的 Go 文件都必须运行 `gofmt`。标准库或现有依赖足以解决问题
  时，不要引入新依赖。
- 新增或重命名配置字段时，同步更新 `sd_config/config.go`、
  `sd_config/config.example.json` 和 README 中的相关示例，并确保非目标
  环境不受影响。

## Go 编码规范

- 函数总长度超过 20 行时，在函数定义上方添加一句简洁的职责说明。
- 文件级 `const` 和 `var` 声明原则上放在 import 之后、其他声明之前。
- 如果 `const` 或 `var` 的声明依赖当前文件定义的自定义类型，必须先定义该
  类型，再定义依赖它的 `const` 或 `var`。例如枚举类型应位于对应的
  `iota` 常量组之前。
- 每个 struct 定义后应紧跟该 struct 的构造函数和全部方法。多个 struct
  按“struct、构造函数、方法”的完整代码块依次排列。
- 与 struct 无关的普通辅助函数放在相关 struct 代码块之后，避免打断
  struct、构造函数和方法的连续性。
- 仅在确实需要跨包调用时导出 Struct、Interface、Func、Method、Const、
  Var 和字段；只在包内使用的声明统一使用小写名称。
- 私有 struct 实现跨包公开接口时，接口要求的方法必须保持导出名称，以
  满足 Go 的跨包接口实现规则。
- 平台对应实现（Linux/Darwin）应保持命名、声明顺序和控制流结构对称。

## 单元测试规范

- 每个生产函数或方法必须只有一个独立的 Go 测试函数，不得将多个生产函数的
  测试集中到 `TestAll...` 等聚合测试中，也不将单个函数的测试拆分成多个测试函数。
- 测试必须放在被测源文件对应的测试文件中：`xxx.go` 对应
  `xxx_test.go`，平台文件 `xxx_linux.go`、`xxx_darwin.go` 分别对应
  `xxx_linux_test.go`、`xxx_darwin_test.go`。
- 每个测试函数采用可嵌套的 GoConvey 结构：

  ```go
  func TestFunctionConvey(t *testing.T) {
      convey.Convey("Test Function", t, func() {
          convey.Convey("success", func() {
              // Arrange, Act, Assert
          })
      })
  }
  ```

- 一个生产函数对应一个根 Convey；成功、边界和错误路径使用该根 Convey
  下的子 Convey 表达。禁止用一个根 Convey 包裹多个生产函数的测试。
- 外部依赖、系统调用、文件、网络及难以稳定构造的行为使用 gomonkey
  隔离；纯函数和可确定构造的状态逻辑直接测试，不为使用 Mock 而 Mock。
- gomonkey 测试必须使用禁用优化和内联的参数执行：

  ```bash
  go test -v -gcflags="all=-l -N" ./path/to/pkg
  ```

- 不删除或破坏无关的原有测试。新增测试应覆盖有效行为，并使用明确断言，
  禁止空壳测试或只验证“不 panic”而不验证结果。
- 平台测试需要在本机执行对应平台用例，并至少对另一平台执行交叉编译测试。

## 构建与测试

在 macOS 中：

```bash
go test -gcflags="all=-l -N" ./...
go test -race ./...
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet ./...
make build-darwin
```

在 Linux 环境中：

```bash
go test -gcflags="all=-l -N" ./...
go test -race ./...
make build-linux
```

常用 Make 目标：

```bash
make test
make test-race
make build
make build-linux
make build-darwin DARWIN_ARCH=arm64
```

`make build` 按当前 `GOOS` 构建 `stevedore`；`make build-linux` 固定生成
Linux amd64 二进制。`make build-darwin` 默认使用当前 Go 工具链的架构，
也可通过 `DARWIN_ARCH=amd64` 或 `arm64` 指定。

## 打包与运行

`make all` 依次执行测试、清理输出目录、按当前 `GOOS` 构建和打包。
`make package` 会将仓库根目录下的 `stevedore` 移动到
`output/stevedore/bin/`，将示例配置复制到 `output/stevedore/etc/`，
并生成 `output/stevedore.tgz`。单独执行 `make package` 前必须先生成
名为 `stevedore` 的二进制。

运行打包后的程序：

```bash
cd output/stevedore
bin/stevedore -c etc/config.example.json -t
bin/stevedore -c etc/config.example.json
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
- CHash key 提取失败时遵循 `InvalidKeyPolicy`，固定区间和长度字段均已做
  payload 边界检查。
- 热更新拒绝所有非 `Upload` 变化；候选配置失败或 Reactor 切换失败时不
  破坏当前代，成功切换后旧 Registry 会被关闭。
- Darwin 上不会为 `ListenParallel > 1` 创建冗余 Reactor。
- 使用上述命令通过本机测试、race 测试和 Linux 交叉构建。
