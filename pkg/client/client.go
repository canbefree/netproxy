package client

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"sync"

	"netproxy/pkg/common"
	"netproxy/pkg/config"
)

// Client 客户端实例
type Client struct {
	config     *config.Config
	conn       net.Conn
	proxies    map[string]*LocalProxy
	proxiesMux sync.Mutex
}

// LocalProxy 本地代理
type LocalProxy struct {
	Name       string
	Type       string
	LocalAddr  string
	RemotePort int
	client     *Client
	proxyID    uint32
	connMap    map[uint32]net.Conn
	connMapMux sync.Mutex
}

// NewClient 创建新的客户端实例
func NewClient(config *config.Config) *Client {
	return &Client{
		config:  config,
		proxies: make(map[string]*LocalProxy),
	}
}

// Start 启动客户端
func (c *Client) Start() error {
	// 连接到服务端
	var err error
	c.conn, err = net.Dial("tcp", c.config.Client.ServerAddr)
	if err != nil {
		return err
	}

	common.Log("Connected to server at %s", c.config.Client.ServerAddr)

	// 认证
	err = common.Authenticate(c.conn, c.config.Client.Token)
	if err != nil {
		return err
	}

	common.Log("Authenticated successfully")

	// 启动心跳协程
	go common.HeartbeatLoop(c.conn)

	// 创建所有代理
	for _, proxyConfig := range c.config.Client.Proxies {
		proxy := &LocalProxy{
			Name:       proxyConfig.Name,
			Type:       proxyConfig.Type,
			LocalAddr:  proxyConfig.LocalAddr,
			RemotePort: proxyConfig.RemotePort,
			client:     c,
			connMap:    make(map[uint32]net.Conn),
		}

		// 添加到代理列表
		c.proxiesMux.Lock()
		c.proxies[proxyConfig.Name] = proxy
		c.proxiesMux.Unlock()

		// 注册代理
		err = proxy.register()
		if err != nil {
			common.Log("Error registering proxy %s: %v", proxyConfig.Name, err)
			continue
		}

		common.Log("Proxy %s registered successfully", proxyConfig.Name)
	}

	// 处理服务端消息
	for {
		header, data, err := common.RecvMsg(c.conn)
		if err != nil {
			common.Log("Error receiving message: %v", err)
			break
		}

		// 根据消息类型处理
		c.handleMsg(header, data)
	}

	// 关闭连接
	c.conn.Close()

	common.Log("Disconnected from server")

	return nil
}

// handleMsg 处理服务端消息
func (c *Client) handleMsg(header common.MsgHeader, data []byte) {
	switch header.Type {
	case common.MsgTypeNewProxy:
		// 处理代理ID分配
		c.handleNewProxy(header, data)
	case common.MsgTypeProxyData:
		// 处理代理数据
		c.handleProxyData(header, data)
	case common.MsgTypeProxyClose:
		// 处理代理关闭
		c.handleProxyClose(header)
	case common.MsgTypeHeartbeat:
		// 处理心跳消息
		// 不需要特殊处理，保持连接即可
	default:
		common.Log("Unknown message type: %d", header.Type)
	}
}

// handleNewProxy 处理代理ID分配
func (c *Client) handleNewProxy(header common.MsgHeader, data []byte) {
	// 解析代理ID
	var proxyID uint32
	err := binary.Read(bytes.NewReader(data), binary.BigEndian, &proxyID)
	if err != nil {
		common.Log("Error parsing proxy ID: %v", err)
		return
	}

	// 查找第一个未分配ID的代理
	var proxy *LocalProxy
	c.proxiesMux.Lock()
	for _, p := range c.proxies {
		if p.proxyID == 0 {
			proxy = p
			break
		}
	}
	c.proxiesMux.Unlock()

	if proxy != nil {
		proxy.proxyID = proxyID
		common.Log("Proxy %s assigned ID: %d", proxy.Name, proxyID)
	}
}

// handleProxyData 处理代理数据
func (c *Client) handleProxyData(header common.MsgHeader, data []byte) {
	// 查找代理
	var proxy *LocalProxy
	c.proxiesMux.Lock()
	for _, p := range c.proxies {
		if p.proxyID != 0 {
			proxy = p
			break
		}
	}
	c.proxiesMux.Unlock()

	if proxy == nil {
		common.Log("No proxy found for message")
		return
	}

	// 处理TCP代理数据
	if proxy.Type == "tcp" {
		// 查找连接
		proxy.connMapMux.Lock()
		conn, ok := proxy.connMap[header.ProxyID]
		proxy.connMapMux.Unlock()

		if !ok {
			// 这是一个新的连接请求
			proxy.handleNewConn(header.ProxyID)
			return
		}

		// 转发数据
		_, err := conn.Write(data)
		if err != nil {
			common.Log("Error writing to local connection: %v", err)
			proxy.closeConn(header.ProxyID)
		}
	}
}

// handleProxyClose 处理代理关闭
func (c *Client) handleProxyClose(header common.MsgHeader) {
	// 查找代理
	var proxy *LocalProxy
	c.proxiesMux.Lock()
	for _, p := range c.proxies {
		if p.proxyID != 0 {
			proxy = p
			break
		}
	}
	c.proxiesMux.Unlock()

	if proxy == nil {
		common.Log("No proxy found for close message")
		return
	}

	// 关闭连接
	proxy.closeConn(header.ProxyID)
}

// register 注册代理
func (p *LocalProxy) register() error {
	// 创建新代理消息
	newProxyMsg := common.NewProxyMsg{
		Name:       p.Name,
		Type:       p.Type,
		LocalAddr:  p.LocalAddr,
		RemotePort: p.RemotePort,
	}

	// 序列化消息
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, &newProxyMsg)

	// 发送消息到服务端
	return common.SendMsg(p.client.conn, common.MsgTypeNewProxy, 0, buf.Bytes())
}

// handleNewConn 处理新连接请求
func (p *LocalProxy) handleNewConn(connID uint32) {
	// 连接到本地服务
	localConn, err := net.Dial("tcp", p.LocalAddr)
	if err != nil {
		common.Log("Error connecting to local service %s: %v", p.LocalAddr, err)
		// 通知服务端连接失败
		common.SendMsg(p.client.conn, common.MsgTypeProxyClose, connID, nil)
		return
	}

	// 保存连接
	p.connMapMux.Lock()
	p.connMap[connID] = localConn
	p.connMapMux.Unlock()

	defer func() {
		// 关闭连接
		localConn.Close()

		// 从连接列表中移除
		p.closeConn(connID)
	}()

	// 转发数据
	buf := make([]byte, 4096)
	for {
		n, err := localConn.Read(buf)
		if err != nil {
			if err != io.EOF {
				common.Log("Error reading from local service: %v", err)
			}
			// 通知服务端连接关闭
			common.SendMsg(p.client.conn, common.MsgTypeProxyClose, connID, nil)
			break
		}

		// 发送数据到服务端
		err = common.SendMsg(p.client.conn, common.MsgTypeProxyData, connID, buf[:n])
		if err != nil {
			common.Log("Error sending data to server: %v", err)
			break
		}
	}
}

// closeConn 关闭连接
func (p *LocalProxy) closeConn(connID uint32) {
	p.connMapMux.Lock()
	defer p.connMapMux.Unlock()

	if conn, ok := p.connMap[connID]; ok {
		conn.Close()
		delete(p.connMap, connID)
	}
}
