package common

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// 消息类型常量
const (
	MsgTypeAuth       = 1 // 认证消息
	MsgTypeNewProxy   = 2 // 新代理请求
	MsgTypeProxyData  = 3 // 代理数据
	MsgTypeProxyClose = 4 // 代理关闭
	MsgTypeHeartbeat  = 5 // 心跳消息
)

// 消息头结构
type MsgHeader struct {
	Type    uint8  // 消息类型
	Length  uint32 // 消息长度
	ProxyID uint32 // 代理ID
}

// 认证消息结构
type AuthMsg struct {
	Token string
}

// 发送认证信息（客户端调用）
func SendAuth(conn net.Conn, token string) error {
	// 构建认证消息：先写入令牌长度，再写入令牌内容
	var buf bytes.Buffer

	// 写入令牌长度
	tokenLen := uint32(len(token))
	err := binary.Write(&buf, binary.BigEndian, tokenLen)
	if err != nil {
		return err
	}

	// 写入令牌内容
	_, err = buf.Write([]byte(token))
	if err != nil {
		return err
	}

	// 发送认证消息
	return SendMsg(conn, MsgTypeAuth, 0, buf.Bytes())
}

// 接收并验证认证信息（服务端调用）
func RecvAuth(conn net.Conn, expectedToken string) error {
	// 接收认证消息
	header, data, err := RecvMsg(conn)
	if err != nil {
		return err
	}

	if header.Type != MsgTypeAuth {
		return errors.New("expected auth message")
	}

	// 解析令牌：先读取令牌长度，再读取令牌内容
	reader := bytes.NewReader(data)

	// 读取令牌长度
	var tokenLen uint32
	err = binary.Read(reader, binary.BigEndian, &tokenLen)
	if err != nil {
		return err
	}

	// 读取令牌内容
	tokenBytes := make([]byte, tokenLen)
	_, err = reader.Read(tokenBytes)
	if err != nil {
		return err
	}

	token := string(tokenBytes)

	// 验证令牌
	if token != expectedToken {
		return errors.New("invalid token")
	}

	// 发送认证成功响应
	return SendMsg(conn, MsgTypeAuth, 0, nil)
}

// 新代理消息结构
type NewProxyMsg struct {
	Name       string
	Type       string
	LocalAddr  string
	RemotePort int32
}

// 发送新代理消息（客户端调用）
func SendNewProxyMsg(conn net.Conn, msg NewProxyMsg) error {
	// 构建消息：依次写入各字段，字符串采用"长度+内容"的方式
	var buf bytes.Buffer

	// 写入Name字段
	nameLen := uint32(len(msg.Name))
	binary.Write(&buf, binary.BigEndian, nameLen)
	buf.Write([]byte(msg.Name))

	// 写入Type字段
	typeLen := uint32(len(msg.Type))
	binary.Write(&buf, binary.BigEndian, typeLen)
	buf.Write([]byte(msg.Type))

	// 写入LocalAddr字段
	localAddrLen := uint32(len(msg.LocalAddr))
	binary.Write(&buf, binary.BigEndian, localAddrLen)
	buf.Write([]byte(msg.LocalAddr))

	// 写入RemotePort字段（使用固定大小的int32类型）
	binary.Write(&buf, binary.BigEndian, msg.RemotePort)

	// 发送消息
	return SendMsg(conn, MsgTypeNewProxy, 0, buf.Bytes())
}

// 接收新代理消息（服务端调用）
func RecvNewProxyMsg(data []byte) (NewProxyMsg, error) {
	var msg NewProxyMsg
	reader := bytes.NewReader(data)

	// 读取Name字段
	var nameLen uint32
	if err := binary.Read(reader, binary.BigEndian, &nameLen); err != nil {
		return msg, err
	}
	nameBytes := make([]byte, nameLen)
	if _, err := reader.Read(nameBytes); err != nil {
		return msg, err
	}
	msg.Name = string(nameBytes)

	// 读取Type字段
	var typeLen uint32
	if err := binary.Read(reader, binary.BigEndian, &typeLen); err != nil {
		return msg, err
	}
	typeBytes := make([]byte, typeLen)
	if _, err := reader.Read(typeBytes); err != nil {
		return msg, err
	}
	msg.Type = string(typeBytes)

	// 读取LocalAddr字段
	var localAddrLen uint32
	if err := binary.Read(reader, binary.BigEndian, &localAddrLen); err != nil {
		return msg, err
	}
	localAddrBytes := make([]byte, localAddrLen)
	if _, err := reader.Read(localAddrBytes); err != nil {
		return msg, err
	}
	msg.LocalAddr = string(localAddrBytes)

	// 读取RemotePort字段
	if err := binary.Read(reader, binary.BigEndian, &msg.RemotePort); err != nil {
		return msg, err
	}

	return msg, nil
}

// 发送消息
func SendMsg(conn net.Conn, msgType uint8, proxyID uint32, data []byte) error {
	// 构建消息头
	header := MsgHeader{
		Type:    msgType,
		Length:  uint32(len(data)),
		ProxyID: proxyID,
	}

	// 序列化消息头
	headerBuf := new(bytes.Buffer)
	err := binary.Write(headerBuf, binary.BigEndian, header)
	if err != nil {
		return err
	}

	// 发送消息头和数据
	_, err = conn.Write(headerBuf.Bytes())
	if err != nil {
		return err
	}

	if len(data) > 0 {
		_, err = conn.Write(data)
		if err != nil {
			return err
		}
	}

	return nil
}

// 接收消息
func RecvMsg(conn net.Conn) (MsgHeader, []byte, error) {
	// 读取消息头
	headerBuf := make([]byte, 9) // 1+4+4=9
	_, err := io.ReadFull(conn, headerBuf)
	if err != nil {
		return MsgHeader{}, nil, err
	}

	// 解析消息头
	var header MsgHeader
	headerReader := bytes.NewReader(headerBuf)
	err = binary.Read(headerReader, binary.BigEndian, &header)
	if err != nil {
		return MsgHeader{}, nil, err
	}

	// 读取消息数据
	data := make([]byte, header.Length)
	if header.Length > 0 {
		_, err = io.ReadFull(conn, data)
		if err != nil {
			return MsgHeader{}, nil, err
		}
	}

	return header, data, nil
}

// 处理TCP转发
func HandleTCPForward(localConn, remoteConn net.Conn) {
	defer localConn.Close()
	defer remoteConn.Close()

	// 创建双向转发
	done := make(chan struct{})

	// 从localConn读取数据发送到remoteConn
	go func() {
		io.Copy(remoteConn, localConn)
		done <- struct{}{}
	}()

	// 从remoteConn读取数据发送到localConn
	go func() {
		io.Copy(localConn, remoteConn)
		done <- struct{}{}
	}()

	// 等待任一方向结束
	<-done
}

// 认证函数（保留旧接口用于向后兼容）
func Authenticate(conn net.Conn, expectedToken string) error {
	return RecvAuth(conn, expectedToken)
}

// 发送心跳
func SendHeartbeat(conn net.Conn) error {
	return SendMsg(conn, MsgTypeHeartbeat, 0, nil)
}

// 心跳协程
func HeartbeatLoop(conn net.Conn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := SendHeartbeat(conn); err != nil {
				return
			}
		}
	}
}

// 日志函数
func Log(format string, args ...interface{}) {
	timeStr := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("[%s] %s\n", timeStr, fmt.Sprintf(format, args...))
}
