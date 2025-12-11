package service

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"netproxy/pkg/common"
	"netproxy/pkg/config"
	"strconv"
	"sync"
)

// Server 服务端实例
type Server struct {
	config      *config.Config
	listener    net.Listener
	clients     map[*ClientConn]bool
	clientsMux  sync.Mutex
	proxies     map[uint32]*Proxy
	proxiesMux  sync.Mutex
	nextProxyID uint32
}

// ClientConn 客户端连接
type ClientConn struct {
	conn    net.Conn
	server  *Server
	proxies map[uint32]*Proxy
}

// Proxy 代理实例
type Proxy struct {
	ID         uint32
	Name       string
	Type       string
	LocalAddr  string
	RemotePort int
	ClientConn *ClientConn
	listener   net.Listener
	conns      map[uint32]net.Conn
	connMux    sync.Mutex
}

// NewServer 创建新的服务端实例
func NewServer(config *config.Config) *Server {
	return &Server{
		config:      config,
		clients:     make(map[*ClientConn]bool),
		proxies:     make(map[uint32]*Proxy),
		nextProxyID: 1,
	}
}

// Start 启动服务端
func (s *Server) Start() error {
	// 监听服务端端口
	var err error
	s.listener, err = net.Listen("tcp", s.config.Server.ListenAddr)
	if err != nil {
		return err
	}

	common.Log("Server listening on %s", s.config.Server.ListenAddr)

	// 接受客户端连接
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			common.Log("Error accepting connection: %v", err)
			continue
		}

		// 处理客户端连接
		go s.handleClient(conn)
	}
}

// handleClient 处理客户端连接
func (s *Server) handleClient(conn net.Conn) {
	common.Log("New client connected from %s", conn.RemoteAddr().String())

	// 创建客户端连接实例
	clientConn := &ClientConn{
		conn:    conn,
		server:  s,
		proxies: make(map[uint32]*Proxy),
	}

	// 添加到客户端列表
	s.clientsMux.Lock()
	s.clients[clientConn] = true
	s.clientsMux.Unlock()

	defer func() {
		// 关闭连接
		conn.Close()

		// 从客户端列表移除
		s.clientsMux.Lock()
		delete(s.clients, clientConn)
		s.clientsMux.Unlock()

		// 清理代理
		clientConn.cleanupProxies()

		common.Log("Client disconnected from %s", conn.RemoteAddr().String())
	}()

	// 认证客户端
	err := common.Authenticate(conn, s.config.Server.Token)
	if err != nil {
		common.Log("Authentication failed: %v", err)
		return
	}

	common.Log("Client authenticated successfully")

	// 启动心跳协程
	go common.HeartbeatLoop(conn)

	// 处理客户端消息
	for {
		header, data, err := common.RecvMsg(conn)
		if err != nil {
			common.Log("Error receiving message: %v", err)
			break
		}

		// 根据消息类型处理
		s.handleMsg(clientConn, header, data)
	}
}

// handleMsg 处理客户端消息
func (s *Server) handleMsg(clientConn *ClientConn, header common.MsgHeader, data []byte) {
	switch header.Type {
	case common.MsgTypeNewProxy:
		// 处理新代理请求
		s.handleNewProxy(clientConn, data)
	case common.MsgTypeProxyData:
		// 处理代理数据
		s.handleProxyData(clientConn, header, data)
	case common.MsgTypeProxyClose:
		// 处理代理关闭
		s.handleProxyClose(clientConn, header)
	case common.MsgTypeHeartbeat:
		// 处理心跳消息
		// 不需要特殊处理，保持连接即可
	default:
		common.Log("Unknown message type: %d", header.Type)
	}
}

// handleNewProxy 处理新代理请求
func (s *Server) handleNewProxy(clientConn *ClientConn, data []byte) {
	// 解析新代理消息
	var newProxyMsg common.NewProxyMsg
	err := binary.Read(bytes.NewReader(data), binary.BigEndian, &newProxyMsg)
	if err != nil {
		common.Log("Error parsing new proxy message: %v", err)
		return
	}

	// 创建新代理
	proxy := &Proxy{
		ID:         s.nextProxyID,
		Name:       newProxyMsg.Name,
		Type:       newProxyMsg.Type,
		LocalAddr:  newProxyMsg.LocalAddr,
		RemotePort: newProxyMsg.RemotePort,
		ClientConn: clientConn,
		conns:      make(map[uint32]net.Conn),
	}
	s.nextProxyID++

	// 启动代理监听
	err = proxy.start()
	common.Log("Proxy %s started on local addr %s", newProxyMsg.Name, newProxyMsg.LocalAddr)
	if err != nil {
		common.Log("Error starting proxy %s: %v", newProxyMsg.Name, err)
		return
	}

	// 添加到代理列表
	s.proxiesMux.Lock()
	s.proxies[proxy.ID] = proxy
	s.proxiesMux.Unlock()

	// 添加到客户端代理列表
	clientConn.proxies[proxy.ID] = proxy

	common.Log("Proxy %s started on remote port %d", newProxyMsg.Name, newProxyMsg.RemotePort)

	// 发送代理ID给客户端
	proxyIDBuf := new(bytes.Buffer)
	binary.Write(proxyIDBuf, binary.BigEndian, proxy.ID)
	common.SendMsg(clientConn.conn, common.MsgTypeNewProxy, 0, proxyIDBuf.Bytes())
}

// handleProxyData 处理代理数据
func (s *Server) handleProxyData(clientConn *ClientConn, header common.MsgHeader, data []byte) {
	// 查找代理
	s.proxiesMux.Lock()
	proxy, ok := s.proxies[header.ProxyID]
	s.proxiesMux.Unlock()

	if !ok {
		common.Log("Proxy not found: %d", header.ProxyID)
		return
	}

	// 处理TCP代理数据
	if proxy.Type == "tcp" {
		// 查找连接
		proxy.connMux.Lock()
		conn, ok := proxy.conns[header.ProxyID]
		proxy.connMux.Unlock()

		if !ok {
			// 这是一个新的连接请求
			proxy.handleNewConn(header.ProxyID, data)
			return
		}

		// 转发数据
		_, err := conn.Write(data)
		if err != nil {
			common.Log("Error writing to connection: %v", err)
			proxy.closeConn(header.ProxyID)
		}
	}
}

// handleProxyClose 处理代理关闭
func (s *Server) handleProxyClose(clientConn *ClientConn, header common.MsgHeader) {
	// 查找代理
	s.proxiesMux.Lock()
	proxy, ok := s.proxies[header.ProxyID]
	s.proxiesMux.Unlock()

	if !ok {
		common.Log("Proxy not found: %d", header.ProxyID)
		return
	}

	// 关闭代理
	proxy.close()

	// 从列表中移除
	s.proxiesMux.Lock()
	delete(s.proxies, proxy.ID)
	s.proxiesMux.Unlock()

	// 从客户端代理列表中移除
	delete(clientConn.proxies, proxy.ID)

	common.Log("Proxy %s closed", proxy.Name)
}

// cleanupProxies 清理客户端的所有代理
func (c *ClientConn) cleanupProxies() {
	for _, proxy := range c.proxies {
		proxy.close()
	}
}

// start 启动代理监听
func (p *Proxy) start() error {
	if p.Type != "tcp" {
		return nil
	}

	// 初始化连接映射
	p.conns = make(map[uint32]net.Conn)

	// 监听远程端口
	addr := net.JoinHostPort("0.0.0.0", strconv.Itoa(p.RemotePort))
	common.Log("Proxy %s listening on %s", p.Name, addr)

	var err error
	p.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	// 接受连接
	go func() {
		for {
			conn, err := p.listener.Accept()
			if err != nil {
				common.Log("Error accepting connection for proxy %s: %v", p.Name, err)
				break
			}

			// 处理新连接
			p.handleRemoteConn(conn)
		}
	}()

	return nil
}

// handleRemoteConn 处理远程连接
func (p *Proxy) handleRemoteConn(conn net.Conn) {
	// 生成连接ID
	connID := p.ClientConn.server.nextProxyID
	p.ClientConn.server.nextProxyID++

	// 保存连接
	p.connMux.Lock()
	p.conns[connID] = conn
	p.connMux.Unlock()

	defer func() {
		// 关闭连接
		conn.Close()

		// 从连接列表中移除
		p.closeConn(connID)
	}()

	// 通知客户端新连接
	common.SendMsg(p.ClientConn.conn, common.MsgTypeProxyData, connID, nil)

	// 转发数据
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				common.Log("Error reading from remote connection: %v", err)
			}
			// 通知客户端连接关闭
			common.SendMsg(p.ClientConn.conn, common.MsgTypeProxyClose, connID, nil)
			break
		}

		// 发送数据到客户端
		err = common.SendMsg(p.ClientConn.conn, common.MsgTypeProxyData, connID, buf[:n])
		if err != nil {
			common.Log("Error sending data to client: %v", err)
			break
		}
	}
}

// handleNewConn 处理新的连接请求
func (p *Proxy) handleNewConn(connID uint32, data []byte) {
	// 连接到本地服务
	localConn, err := net.Dial("tcp", p.LocalAddr)
	if err != nil {
		common.Log("Error connecting to local service %s: %v", p.LocalAddr, err)
		// 通知客户端连接失败
		common.SendMsg(p.ClientConn.conn, common.MsgTypeProxyClose, connID, nil)
		return
	}

	// 保存连接
	p.connMux.Lock()
	p.conns[connID] = localConn
	p.connMux.Unlock()

	defer func() {
		// 关闭连接
		localConn.Close()

		// 从连接列表中移除
		p.closeConn(connID)
	}()

	// 如果有初始数据，先发送
	if len(data) > 0 {
		_, err = localConn.Write(data)
		if err != nil {
			common.Log("Error writing initial data to local service: %v", err)
			// 通知客户端连接关闭
			common.SendMsg(p.ClientConn.conn, common.MsgTypeProxyClose, connID, nil)
			return
		}
	}

	// 转发数据
	buf := make([]byte, 4096)
	for {
		n, err := localConn.Read(buf)
		if err != nil {
			if err != io.EOF {
				common.Log("Error reading from local service: %v", err)
			}
			// 通知客户端连接关闭
			common.SendMsg(p.ClientConn.conn, common.MsgTypeProxyClose, connID, nil)
			break
		}

		// 发送数据到客户端
		err = common.SendMsg(p.ClientConn.conn, common.MsgTypeProxyData, connID, buf[:n])
		if err != nil {
			common.Log("Error sending data to client: %v", err)
			break
		}
	}
}

// closeConn 关闭连接
func (p *Proxy) closeConn(connID uint32) {
	p.connMux.Lock()
	defer p.connMux.Unlock()

	if conn, ok := p.conns[connID]; ok {
		conn.Close()
		delete(p.conns, connID)
	}
}

// close 关闭代理
func (p *Proxy) close() {
	if p.listener != nil {
		p.listener.Close()
	}

	// 关闭所有连接
	p.connMux.Lock()
	for _, conn := range p.conns {
		conn.Close()
	}
	p.conns = make(map[uint32]net.Conn)
	p.connMux.Unlock()
}

// 添加代理连接映射
func init() {
	// 为Proxy添加连接映射
	binary.Size(Proxy{})
}
