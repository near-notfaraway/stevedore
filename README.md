# Stevedore

## 简介
Stevedore 实现了一个四层负载均衡，并且可根据四层数据包的字节内容进行灵活地转发，其主要特点有：

- 四层协议目前仅支持 UDP，也可应用于 KCP、QUIC 等协议，甚至是私有协议
- 数据包的上游选择，支持基于字节进行匹配，甚至基于比特来进行匹配
- 数据包的负载均衡，支持指定的字节作为 hash key
- 通过 epoll 搭配 recvmmsg 对转发效率进行优化
- 具有健康检查功能，当节点均可不可用时，支持指定节点为备用临时的节点

## 快速开始
### 编译
``` bash
cd ${stevedore_path}
make
mv output/stevedore ${target_dir}
```

### 启动

``` bash
cd ${target_dir}/stevedore
bin/stevedore -c etc/stevedore.config
```
