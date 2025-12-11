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

// 新代理消息结构
type NewProxyMsg struct {
	Name       string
	Type       string
	LocalAddr  string
	RemotePort int
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

// 认证函数
func Authenticate(conn net.Conn, expectedToken string) error {
	// 接收认证消息
	header, data, err := RecvMsg(conn)
	if err != nil {
		return err
	}

	if header.Type != MsgTypeAuth {
		return errors.New("expected auth message")
	}

	var authMsg AuthMsg
	if len(data) > 0 {
		// 解析认证消息
		if err := binary.Read(bytes.NewReader(data), binary.BigEndian, &authMsg.Token); err != nil {
			return err
		}
	}

	// 验证令牌
	if authMsg.Token != expectedToken {
		return errors.New("invalid token")
	}

	// 发送认证成功响应
	return SendMsg(conn, MsgTypeAuth, 0, nil)
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
