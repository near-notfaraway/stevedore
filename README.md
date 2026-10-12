# Stevedore

## 简介
Stevedore 实现了一个四层负载均衡，并且可根据四层数据包的字节内容进行灵活地转发，其主要特点有：

- 四层协议目前支持 UDP，也可应用于 KCP、QUIC、TFTP 协议，或基于 UDP 的一些私有协议
- 支持 IPv4、IPv6 UDP 监听和后端转发，同一 Session 可访问不同地址族的后端
- 数据包的上游选择，支持基于字节进行匹配，甚至基于比特来进行匹配
- 数据包的负载均衡，支持组合多个字节区间并通过掩码提取 hash key
- Linux 使用 `epoll`、`recvmmsg` 和 `sendmmsg` 优化转发性能
- macOS 使用 `kqueue` 和兼容的逐包收发路径，便于本地开发与运行
- 具有健康检查功能，支持指定一个正常参与负载均衡、全员不可用时强制兜底的节点

## 平台差异

| 平台 | 事件机制 | UDP 收发 | `ListenParallel` |
| --- | --- | --- | --- |
| Linux | `epoll` + `eventfd` | `recvmmsg` + `sendmmsg` | 按配置启用 `SO_REUSEPORT` 分片 |
| macOS | `kqueue` + `EVFILT_USER` | `recvfrom` + `sendmsg` 兼容批量接口 | 固定为 1，配置大于 1 时记录警告 |

Linux 是生产性能目标。macOS 支持完整功能，但没有 Linux 的批量系统调用和
`SO_REUSEPORT` 多 Reactor 分流能力。

## 快速开始
### 编译

安装 `go.mod` 指定版本的 Go 工具链后执行：

```bash
# 测试当前平台
make test

# 构建 Linux amd64 生产二进制：./stevedore
make build

# 构建当前 Mac 架构：./stevedore-darwin-arm64 或 amd64
make build-darwin

# 指定 Intel Mac
make build-darwin DARWIN_ARCH=amd64

# 测试、构建 Linux amd64 并打包到 output/
make all
```

### 启动

直接构建后：

```bash
./stevedore -config sd_config/config.example.json
```

执行 `make all` 后：

```bash
cd output/stevedore
bin/stevedore -config etc/config.example.json
```

### IPv6 配置

`Server.ListenAddr` 是包含端口的完整监听地址，IPv6 地址必须使用方括号：

```json
"Server": {
  "ListenAddr": "[::]:2614"
}
```

`Peer.IP` 只填写主机地址，不包含方括号或端口。一个 Upstream 可以同时配置
IPv4 和 IPv6 Peer，健康检查与数据转发会分别使用对应地址族的 socket：

```json
"Peers": [
  {
    "IP": "10.0.0.1",
    "Port": 2345,
    "Weight": 1,
    "Backup": false
  },
  {
    "IP": "2001:db8::10",
    "Port": 2345,
    "Weight": 1,
    "Backup": true
  }
]
```

链路本地地址可以在 `IP` 中携带 zone，例如 `"fe80::10%eth0"`。

服务当前只配置一个 `ListenAddr`。`[::]:2614` 是否同时接收 IPv4 流量取决于
操作系统的 `IPV6_V6ONLY` 默认行为，不作为跨平台双栈保证。需要明确同时提供
IPv4、IPv6 入口时，应分别部署监听 `0.0.0.0:2614` 和 `[::]:2614` 的实例。

## 典型使用场景
### 连接 ID 保持
#### 场景说明
对于连接 ID 位于固定字节区间的协议，可以直接从 payload 中提取连接 ID 作为一致性哈希
key。以 KCP 为例，其报文结构如下：

```
0               4   5   6       8 (BYTE)
+---------------+---+---+-------+
|     conv      |cmd|frg|  wnd  |
+---------------+---+---+-------+   8
|     ts        |     sn        |
+---------------+---------------+  16
|     una       |     len       |
+---------------+---------------+  24
|                               |
|        DATA (optional)        |
|                               |
+-------------------------------+
```

`conv` 是位于 `data[0:4]` 的连接 ID。KCP 使用它标识逻辑连接，因此同一 `conv` 的数据包
需要持续转发到同一节点。对于连接 ID 位置或长度可变的协议，需要先确保目标报文具有可供
`KeyBytes` 固定提取的布局。

#### 转发效果
使用 `conv` 作为 `chash` key 后，在可用 Peer 集合不变时，相同 `conv` 会选择相同的后端，
不同 `conv` 可以独立分片。即使同一客户端复用源地址和端口维持多个 KCP 连接，也会按照
各自的 `conv` 正确转发。

![](doc/media/example1-01.jpeg)

#### 配置
``` json
"Upload": {
  "DefaultUpstream": "kcp",         // 默认转发至 kcp Upstream
  "Upstreams": [
    {
      "Name": "kcp",
      "Type": "chash",              // 一致性哈希
      "KeyBytes": "0:4",            // 使用 data[0:4] 作为 hash key
      "InvalidKeyPolicy": "drop",   // payload 过短时丢弃
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
        },
        {
          "IP": "10.0.0.3",
          "Port": 2345,
          "Weight": 1,
          "Backup": false
        }
      ]
    }
  ]
}
```

### 协议版本兼容
#### 场景说明
当新旧版本协议无法由同一组服务完全兼容时，可以根据 payload 中的版本字段选择不同的
上游。假设私有协议的报文结构如下：

```
0               4       6       8 (BYTE)
+---------------+-------+-------+
|     flag      |  ver  |  cmd  |
+---------------+-------+-------+   8
|            session id         |
+---------------+---------------+  16
|     len       |    checksum   |
+---------------+---------------+  24
|                               |
|        DATA (optional)        |
|                               |
+-------------------------------+
```

`ver` 位于 `data[4:6]`，分别使用 `0x0001` 和 `0x0002` 表示两个协议版本。

#### 转发效果
版本 1 和版本 2 的数据包分别转发到 `ver1` 和 `ver2` 集群。其他版本及长度不足的报文
不会命中路由，并由值为 `"drop"` 的默认 Upstream 目标丢弃。

![](doc/media/example2-01.jpeg)

#### 配置
``` json
"Upload": {
  "DefaultUpstream": "drop",        // 未命中任何路由时丢弃
  "Routes": [
    {
      "Name": "protocol-v1",
      "Conditions": [
        {
          "KeyBytes": "4:6",        // 当 data[4:6] == 0x0001 时
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
          "KeyBytes": "4:6",        // 当 data[4:6] == 0x0002 时
          "Operator": "==",
          "Value": "0x0002"
        }
      ],
      "Upstream": "ver2"
    }
  ],
  "Upstreams": [
    {
      "Name": "ver1",
      "Type": "rr",
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
        ...
      ]
    },
    {
      "Name": "ver2",
      "Type": "rr",
      "HealthChecker": {
        "HeartbeatIntervalSec": 5,
        "HeartbeatTimeoutSec": 3,
        "SuccessTimes": 1,
        "FailedTimes": 1
      },
      "Peers": [
        {
          "IP": "10.0.1.1",
          "Port": 2345,
          "Weight": 1,
          "Backup": true
        },
        ...
      ]
    }
  ]
}
```

### 多字段业务分片
#### 场景说明
在物联网、游戏网关等场景中，业务分片可能同时依赖租户 ID 和设备 ID，而这两个字段
并不一定连续。假设报文结构如下：

```
0       2   3   4               8              12 (BYTE)
+-------+---+---+---------------+---------------+
|tenant |flg|cmd|      seq      |   device id   |
+-------+---+---+---------------+---------------+
|                                               |
|                DATA (optional)                |
|                                               |
+-----------------------------------------------+
```

`tenant` 位于 `data[0:2]`，`device id` 位于 `data[8:12]`。

#### 转发效果
`KeyBytes` 按配置顺序拼接多个半开区间，本例使用
`data[0:2] + data[8:12]` 作为 `chash` key。在可用 Peer 集合不变时，同一租户下的
同一设备会稳定落到相同节点。该分片不依赖客户端源地址，因此客户端经过 NAT 或重新绑定
端口后，业务归属仍保持不变。

#### 配置
``` json
"Upload": {
  "DefaultUpstream": "device",
  "Upstreams": [
    {
      "Name": "device",
      "Type": "chash",
      "KeyBytes": "0:2,8:12",        // tenant id + device id
      "InvalidKeyPolicy": "drop",
      "HealthChecker": {
        "HeartbeatIntervalSec": 5,
        "HeartbeatTimeoutSec": 3,
        "SuccessTimes": 1,
        "FailedTimes": 1
      },
      "Peers": [
        {
          "IP": "10.0.2.1",
          "Port": 2345,
          "Weight": 1,
          "Backup": true
        },
        {
          "IP": "10.0.2.2",
          "Port": 2345,
          "Weight": 1,
          "Backup": false
        },
        {
          "IP": "10.0.2.3",
          "Port": 2345,
          "Weight": 1,
          "Backup": false
        }
      ]
    }
  ]
}
```

### 基于标志位的灰度发布
#### 场景说明
协议可以在标志字段中预留一个灰度位。假设 `flag` 位于 `data[2]`，并将其最高位定义为
灰度标记：

```
flag
  7   6   5   4   3   2   1   0 (BIT)
+---+---+---+---+---+---+---+---+
| G |        other flags        |
+---+---+---+---+---+---+---+---+
```

#### 转发效果
当最高位为 1 时，`&=` 规则满足 `(data[2] & 0x80) == 0x80`，数据包会被转发到
`canary` 集群。最高位为 0 或报文长度不足时，数据包不会命中灰度规则，并使用
`DefaultUpstream` 指定的 `stable` 集群。

灰度流量由客户端或业务网关显式标记，不依赖源 IP，也不需要调整监听端口。存在多条路由时，
Stevedore 按配置顺序匹配，并使用首个命中的规则。

#### 配置
``` json
"Upload": {
  "DefaultUpstream": "stable",       // 未命中灰度规则时走稳定集群
  "Routes": [
    {
      "Conditions": [
        {
          "KeyBytes": "2:3",         // 提取 flag 字段
          "Operator": "&=",          // (data[2] & 0x80) == 0x80
          "Value": "0x80"
        }
      ],
      "Upstream": "canary"
    }
  ],
  "Upstreams": [
    {
      "Name": "stable",
      "Type": "rr",
      "HealthChecker": {
        "HeartbeatIntervalSec": 5,
        "HeartbeatTimeoutSec": 3,
        "SuccessTimes": 1,
        "FailedTimes": 1
      },
      "Peers": [
        {
          "IP": "10.0.3.1",
          "Port": 2345,
          "Weight": 1,
          "Backup": true
        },
        {
          "IP": "10.0.3.2",
          "Port": 2345,
          "Weight": 1,
          "Backup": false
        }
      ]
    },
    {
      "Name": "canary",
      "Type": "rr",
      "HealthChecker": {
        "HeartbeatIntervalSec": 5,
        "HeartbeatTimeoutSec": 3,
        "SuccessTimes": 1,
        "FailedTimes": 1
      },
      "Peers": [
        {
          "IP": "10.0.4.1",
          "Port": 2345,
          "Weight": 1,
          "Backup": true
        }
      ]
    }
  ]
}
```

## 配置能力说明

payload 路由支持以下配置：

- 每条路由必须配置非空 `Conditions` 数组，即使只有一个条件。
- 多个 Conditions 按 AND 语义匹配，任一条件不满足时该路由不命中。
- `Mask` 使用 `(payload & Mask) == Value` 语义，仅支持 `==` 和 `!==`。Mask、Value
  和 `KeyBytes` 提取结果的长度必须一致，Value 不能包含 Mask 之外的置位。
- `Name` 用于指标标识；省略时自动生成 `route-<index>`。路由名称不能重复。
- `Upstream` 为空字符串或 `"drop"` 时丢弃；匹配已定义的 Upstream 名称时转发；
  其他值会在启动时作为非法引用拒绝。
- 未命中时使用 `DefaultUpstream`，其取值遵循与路由 `Upstream` 相同的语义。
- 真实 Upstream 名称必须唯一，且不能使用空字符串或 `"drop"`。

例如，以下规则要求魔数和带掩码的 flag 同时匹配：

``` json
{
  "Name": "canary-v1",
  "Conditions": [
    {
      "KeyBytes": "0:2",
      "Operator": "==",
      "Value": "0x1234"
    },
    {
      "KeyBytes": "2:3",
      "Operator": "==",
      "Value": "0x80",
      "Mask": "0xc0"
    }
  ],
  "Upstream": "canary"
}
```

未知 Operator、非法字节区间、重复路由名称以及不存在的 Upstream 引用都会在
启动时直接拒绝。`Log.RouteStatsIntervalSec` 大于 0 时，Server 按该秒数周期输出每条
路由命中数、默认命中数、短报文数和丢弃数的累计日志；值小于等于 0 时关闭统计，不执行
热路径原子计数。路由匹配本身对所有 Operator 均保持零堆分配。

`chash` 的 payload key 支持以下配置：

- `KeyBytes`：一个或多个半开字节区间，多个区间使用逗号分隔并按配置顺序拼接。例如
  `"0:2,4:6"` 提取 `data[0:2] + data[4:6]`。
- `KeyMask`：可选的十六进制或二进制掩码，与拼接后的 key 逐字节按位 AND，长度必须
  与 key 相同。例如 `"0xff0f00f0"`，或单字节位区间掩码 `"0b11110000"`。
- `InvalidKeyPolicy`：payload 长度不足时的处理策略。空值或 `"drop"` 返回无可用 Peer
  并最终丢弃数据包；`"rr"` 回退到同一 Upstream 的加权轮询，但不再保证相同 key
  落到同一节点。

无效区间、负数索引、掩码格式或长度不匹配、未知策略都会在启动时直接拒绝。

每个 Upstream 最多有一个 Peer 可将 `Backup` 设为 `true`。该 Peer 健康时仍按其权重
正常参与 RR/CHash；所有 Peer 都不可用时，无论其当时的健康状态如何，都会将它作为
唯一 Peer 强制兜底。未配置 Backup 时，无可用 Peer 会直接丢包。
