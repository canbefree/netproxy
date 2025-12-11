# NetProxy - 轻量级内网穿透工具

NetProxy是一个用Go语言实现的轻量级内网穿透/反向代理工具，类似于FRP，可用于在任意网络上绕过NAT/防火墙，远程访问内网机器。

## 功能特性

- ✅ TCP协议支持
- ✅ 多代理配置
- ✅ 基于令牌的认证
- ✅ 心跳检测机制
- ✅ 配置文件管理

## 快速开始

### 1. 编译项目

```bash
# 安装依赖
go mod tidy

# 编译服务端和客户端
go build -o netproxy
```

### 2. 配置文件

创建配置文件 `config.json`，可以参考 `config.json.example`：

```json
{
  "server": {
    "listen_addr": "0.0.0.0:8000",
    "token": "your-secret-token"
  },
  "client": {
    "server_addr": "your-server-ip:8000",
    "token": "your-secret-token",
    "proxies": [
      {
        "name": "web",
        "type": "tcp",
        "local_addr": "127.0.0.1:8080",
        "remote_port": 8080
      },
      {
        "name": "ssh",
        "type": "tcp",
        "local_addr": "127.0.0.1:22",
        "remote_port": 2222
      }
    ]
  }
}
```

### 3. 启动服务端

```bash
./netproxy -s -c config.json
```

### 4. 启动客户端

```bash
./netproxy -c -c config.json
```

### 5. 访问服务

现在你可以通过服务端的公网IP和配置的远程端口访问内网服务了：

- Web服务：`http://your-server-ip:8080`
- SSH服务：`ssh -p 2222 user@your-server-ip`

## 命令行参数

```bash
Usage of netproxy:
  -c string
        配置文件路径 (default "config.json")
  -s    启动服务端
  -c    启动客户端
```

## 配置说明

### 服务端配置

- `listen_addr`: 服务端监听地址，格式为 `ip:port`
- `token`: 认证令牌，与客户端保持一致

### 客户端配置

- `server_addr`: 服务端地址，格式为 `ip:port`
- `token`: 认证令牌，与服务端保持一致
- `proxies`: 代理配置列表
  - `name`: 代理名称
  - `type`: 代理类型，目前仅支持 `tcp`
  - `local_addr`: 本地服务地址，格式为 `ip:port`
  - `remote_port`: 服务端的远程端口

## 工作原理

1. 客户端连接到服务端并进行认证
2. 客户端注册需要转发的本地服务
3. 服务端在指定的远程端口上监听连接
4. 当有外部连接访问服务端的远程端口时，服务端将连接信息转发给客户端
5. 客户端连接到本地服务，并在客户端、服务端和本地服务之间建立双向数据转发

## 注意事项

1. 请确保服务端的防火墙已开放监听端口和远程端口
2. 令牌是保证安全性的重要手段，请使用强密码
3. 目前仅支持TCP协议，后续会添加UDP和HTTP支持

## 许可证

MIT License
