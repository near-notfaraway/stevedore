# Stevedore

## 简介
Stevedore 实现了一个四层负载均衡，并且可根据四层数据包的字节内容进行灵活地转发，其主要特点有：

- 四层协议目前支持 UDP，也可应用于 KCP、QUIC、TFTP 协议，或基于 UDP 的一些私有协议
- 数据包的上游选择，支持基于字节进行匹配，甚至基于比特来进行匹配
- 数据包的负载均衡，支持组合多个字节区间并通过掩码提取 hash key
- Linux 使用 `epoll`、`recvmmsg` 和 `sendmmsg` 优化转发性能
- macOS 使用 `kqueue` 和兼容的逐包收发路径，便于本地开发与运行
- 具有健康检查功能，当节点均不可用时，支持启用某个指定为备用的节点

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

## 最佳实践
### 连接 ID 保持
#### 需求
可应用于带有连接 ID 的协议，比如 KCP、QUIC，其中 KCP 报文结构如下：

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

`conv` 表示连接 ID，占用 4 Bytes 长度，用于维持一个通信 session，已实现可靠性传输的各种处理
因此需要保证具有同一连接 ID 的数据包汇集到同一节点

#### 效果
![](doc/media/example1-01.jpeg)

即使对于同一客户端复用端口来维系两个单独的 KCP 连接，也可以被正常进行转发

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

`chash` 的 payload key 支持以下配置：

- `KeyBytes`：一个或多个半开字节区间，多个区间使用逗号分隔并按配置顺序拼接。例如
  `"0:2,4:6"` 提取 `data[0:2] + data[4:6]`。
- `KeyMask`：可选的十六进制或二进制掩码，与拼接后的 key 逐字节按位 AND，长度必须
  与 key 相同。例如 `"0xff0f00f0"`，或单字节位区间掩码 `"0b11110000"`。
- `InvalidKeyPolicy`：payload 长度不足时的处理策略。空值或 `"drop"` 返回无可用 Peer
  并最终丢弃数据包；`"rr"` 回退到同一 Upstream 的加权轮询，但不再保证相同 key
  落到同一节点。

无效区间、负数索引、掩码格式或长度不匹配、未知策略都会在启动时直接拒绝。

### 协议版本兼容
#### 需求
可应用于带有版本的协议，比如对于未实现完全兼容的私有协议，假设私有协议的报文结构如下：

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

在负载均衡的位置，根据数据包的协议版本分发到不同的上游，从而实现协议版本兼容

#### 效果
![](doc/media/example2-01.jpeg)

#### 配置
``` json
"Upload": {
  "DefaultUpstream": "deny",        // 默认丢弃数据包，无此 Upstream
  "Routes": [
    {
      "KeyBytes": "4:6",            // 当 data[0:4] == 0x01 时
      "Operator": "==",             // 转发到 ver1 Upstream
      "Value": "0x01",
      "Upstream": "ver1"
    },
    {
      "KeyBytes": "4:6",            // 当 data[0:4] == 0x02 时
      "Operator": "==",             // 转发到 ver2 Upstream
      "Value": "0x02",
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
