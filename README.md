# Stevedore

Stevedore 是一个面向 UDP 的四层负载均衡器，可根据 payload 内容路由流量，并通过
加权轮询或一致性哈希选择后端。

## 特性

- 支持 IPv4、IPv6 UDP 监听和转发，适用于 KCP、TFTP 及私有 UDP 协议
- 支持按 payload 字节区间、掩码和比特条件选择 Upstream
- 支持加权轮询（RR）和基于 payload key 的一致性哈希（CHash）
- 支持固定区间、多区间拼接、掩码和长度字段等 key 提取方式
- 支持后端健康检查，以及全部后端不可用时的 Backup 节点兜底
- 支持运行时热更新 Routes、Upstreams、Peers 和健康检查配置
- Linux 使用 `epoll`、`recvmmsg` 和 `sendmmsg` 优化转发性能
- macOS 支持完整功能，便于本地开发和验证

```text
UDP client
    |
    v
Routes / DefaultUpstream ---- drop ----> [discard]
    |
    v
RR / CHash
    |
    v
UDP Peer
    |
    +-------- session socket --------> UDP client
```

## 快速开始

项目使用 `go.mod` 指定的 Go 版本。

```bash
# 测试并构建当前平台
make test
make build

# 也可以显式构建 Linux amd64 或当前 Mac 架构
make build-linux
make build-darwin
```

复制并修改示例配置：

```bash
cp sd_config/config.example.json stevedore.config.json
```

启动前先校验配置，再运行服务：

```bash
./stevedore -c stevedore.config.json -t
./stevedore -c stevedore.config.json
```

配置文件支持 JSON、JSONC 和 YAML，YAML 文件扩展名必须为 `.yaml`。执行 `make all`
可以完成测试、构建和打包，产物位于 `output/stevedore/`。

## 配置

完整配置参考：

- [JSON 示例](sd_config/config.example.json)

配置分为以下部分：

| 配置段 | 作用 | 支持热更新 |
| --- | --- | --- |
| `Common` | 工作目录和 PID 文件 | 否 |
| `PProf` | pprof 开关和监听地址 | 否 |
| `Log` | 日志、轮转和统计周期 | 否 |
| `Server` | UDP 监听和收发参数 | 否 |
| `Session` | Session 回收和超时 | 否 |
| `Upload` | Routes、Upstreams、Peers 和健康检查 | 是 |

### 路由和负载均衡

下面的配置将 payload 首字节为 `0x69` 的数据包转发到 `game` Upstream，其他数据包
直接丢弃。同一 `data[1:5]` 会通过 CHash 稳定选择同一 Peer。

```json
{
  "Upload": {
    "DefaultUpstream": "drop",
    "Routes": [
      {
        "Name": "game-traffic",
        "Conditions": [
          {
            "KeyBytes": "0:1",
            "Operator": "==",
            "Value": "0x69"
          }
        ],
        "Upstream": "game"
      }
    ],
    "Upstreams": [
      {
        "Name": "game",
        "Type": "chash",
        "KeyExtractor": {
          "Type": "fixed",
          "Fixed": {
            "KeyBytes": "1:5"
          }
        },
        "InvalidKeyPolicy": "drop",
        "HealthChecker": {
          "HeartbeatIntervalSec": 5,
          "HeartbeatTimeoutSec": 3,
          "SuccessTimes": 1,
          "FailedTimes": 1
        },
        "Peers": [
          {
            "IP": "10.0.0.1",
            "Port": 2345,
            "Weight": 1,
            "Backup": true
          },
          {
            "IP": "10.0.0.2",
            "Port": 2345,
            "Weight": 1,
            "Backup": false
          }
        ]
      }
    ]
  }
}
```

路由规则：

- 多条 `Routes` 按顺序匹配，首个命中的规则生效
- 同一 Route 的多个 `Conditions` 使用 AND 语义
- `Operator` 支持 `==`、`!==`、`&=`、`!&=`、`|=` 和 `!|=`
- `Upstream` 或 `DefaultUpstream` 为 `"drop"` 时丢弃数据包

后端选择：

- `Type: "rr"` 使用 Peer 的 `Weight` 进行加权轮询
- `Type: "chash"` 根据 `KeyExtractor` 提取的 payload key 进行一致性哈希
- `fixed` 支持 `"0:2,8:12"` 形式的多区间拼接和可选 `KeyMask`
- `length_field` 支持从 payload 长度字段提取可变长 key
- CHash key 无效时，`InvalidKeyPolicy` 可选择 `"drop"` 或回退到 `"rr"`

## 典型应用场景

以下示例只展示对应场景的关键配置，完整结构参考
[示例配置](sd_config/config.example.json)。

### 连接 ID 保持

KCP 等协议会在 payload 的固定位置携带连接 ID。同一连接的数据包需要持续转发到同一
后端时，可以提取该字段作为 CHash key。

```text
KCP payload
+---------------+---+---+-------+
| conv (0:4)    |cmd|frg|  wnd  |
+---------------+---+---+-------+
        |
        +----------> CHash ----------> same Peer
```

```json
{
  "Name": "kcp",
  "Type": "chash",
  "KeyExtractor": {
    "Type": "fixed",
    "Fixed": {
      "KeyBytes": "0:4"
    }
  },
  "InvalidKeyPolicy": "drop"
}
```

如果 key 长度由报文字段决定，可以改用 `length_field`：

```json
{
  "Type": "length_field",
  "LengthField": {
    "KeyStart": 4,
    "LengthStart": 1,
    "LengthBytes": 2,
    "ByteOrder": "big",
    "MaxKeyLength": 64
  }
}
```

### 协议版本路由

当不同协议版本由不同集群处理时，可以根据 payload 中的版本字段选择 Upstream。下面的
规则将 `data[4:6]` 为 `0x0001` 和 `0x0002` 的流量分别转发到两个集群，其他版本丢弃。

```text
                          +-- 0x0001 --> ver1
UDP packet --> data[4:6] -+-- 0x0002 --> ver2
                          +-- other  --> drop
```

```json
{
  "DefaultUpstream": "drop",
  "Routes": [
    {
      "Name": "protocol-v1",
      "Conditions": [
        {
          "KeyBytes": "4:6",
          "Operator": "==",
          "Value": "0x0001"
        }
      ],
      "Upstream": "ver1"
    },
    {
      "Name": "protocol-v2",
      "Conditions": [
        {
          "KeyBytes": "4:6",
          "Operator": "==",
          "Value": "0x0002"
        }
      ],
      "Upstream": "ver2"
    }
  ]
}
```

### 多字段业务分片

物联网或游戏网关可能需要同时使用租户 ID 和设备 ID 分片。字段不连续时，可以按配置
顺序拼接多个字节区间作为 CHash key。

```text
payload
  |
  +-- data[0:2]  (tenant ID) --+
  |                            +--> concatenate --> CHash --> Peer
  +-- data[8:12] (device ID) --+
```

```json
{
  "Name": "device",
  "Type": "chash",
  "KeyExtractor": {
    "Type": "fixed",
    "Fixed": {
      "KeyBytes": "0:2,8:12"
    }
  },
  "InvalidKeyPolicy": "drop"
}
```

该分片不依赖客户端源地址，因此客户端经过 NAT 或更换源端口后，业务归属仍保持不变。

### 标志位灰度发布

协议预留灰度标志位时，可以通过按位条件将灰度流量路由到独立集群。下面的规则检查
`data[2]` 的最高位，命中时转发到 `canary`，否则使用 `stable`。

```text
                         +-- (flag & 0x80) == 0x80 --> canary
UDP packet --> data[2] --+
                         +-- otherwise ------------> stable
```

```json
{
  "DefaultUpstream": "stable",
  "Routes": [
    {
      "Name": "canary",
      "Conditions": [
        {
          "KeyBytes": "2:3",
          "Operator": "&=",
          "Value": "0x80"
        }
      ],
      "Upstream": "canary"
    }
  ]
}
```

灰度标记由客户端或业务网关显式设置，不依赖源 IP，也不需要增加监听端口。

## 健康检查

Stevedore 定期向每个 Peer 发送 UDP `heartbeat`。连续失败达到 `FailedTimes` 后将 Peer
移出可选集合，连续成功达到 `SuccessTimes` 后重新加入。

每个 Upstream 最多配置一个 `Backup: true` 的 Peer。它平时正常参与负载均衡；所有
Peer 都不可用时，它将作为唯一兜底节点。未配置 Backup 时，无可用 Peer 的数据包会被丢弃。

## IPv6

监听 IPv6 时，`Server.ListenAddr` 必须包含方括号：

```json
{
  "Server": {
    "ListenAddr": "[::]:2614"
  }
}
```

`Peer.IP` 只填写主机地址，例如 `"2001:db8::10"` 或 `"fe80::10%eth0"`。如需明确同时
提供 IPv4、IPv6 入口，建议分别部署监听 `0.0.0.0:2614` 和 `[::]:2614` 的实例。

## 运行管理

使用相同配置文件向运行中的进程发送信号：

```bash
# 重新加载 Upload 配置
./stevedore -c stevedore.config.json -s reload

# 优雅退出
./stevedore -c stevedore.config.json -s stop
./stevedore -c stevedore.config.json -s quit
```

`reload` 只允许 `Upload` 段发生变化。新配置校验或切换失败时，服务继续使用原配置；
最终结果以运行进程日志中的 `reload config succeeded` 或 `reload config failed` 为准。

Stevedore 默认以前台方式运行，并通过 `Common.PidPath` 持有 PID 文件锁。生产环境建议
交由 systemd、Supervisor 或容器运行时管理，并为 `Common.PidPath` 配置绝对路径。

## 平台差异

| 平台 | 事件机制 | UDP 收发 | `ListenParallel` |
| --- | --- | --- | --- |
| Linux | `epoll` + `eventfd` | `recvmmsg` + `sendmmsg` | 支持 `SO_REUSEPORT` 分片 |
| macOS | `kqueue` + `EVFILT_USER` | `recvfrom` + `sendmsg` | 固定为 1 |

Linux 是生产性能目标。macOS 主要用于本地开发与功能验证。
