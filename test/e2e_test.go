package test

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	c "wtt/internal/client"
	s "wtt/internal/server"
)

func TestE2E(t *testing.T) {
	server, err := s.NewServer(s.ServerConfig{})
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	service, err := c.NewService(c.ServiceConfig{})
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	consumer, err := c.NewConsumer(c.ConsumerConfig{})
	if err != nil {
		t.Fatalf("Failed to create consumer: %v", err)
	}

	// 避免未使用变量错误
	_ = server
	_ = service
	_ = consumer
}

// 新增端到端测试函数，使用 TCP 传输 1MB 数据，并校验数据完整性
func TestTCPDataTransfer(t *testing.T) {
	const dataSize = 1024 * 1024 // 1MB
	// 构造测试数据
	sentData := bytes.Repeat([]byte("A"), dataSize)

	// 启动 TCP 服务器，监听随机端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	done := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("Failed to accept connection: %v", err)
			return
		}
		defer conn.Close()

		// 读取全部数据
		buf := make([]byte, dataSize)
		n, err := io.ReadFull(conn, buf)
		if err != nil {
			t.Errorf("Failed reading from connection: %v", err)
			return
		}
		if n != dataSize {
			t.Errorf("Expected %d bytes, got %d", dataSize, n)
			return
		}

		// 将数据回写给客户端
		_, err = conn.Write(buf)
		if err != nil {
			t.Errorf("Failed writing to connection: %v", err)
			return
		}
		close(done)
	}()

	// 作为客户端连接服务器
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial server: %v", err)
	}
	defer conn.Close()

	// 发送数据
	n, err := conn.Write(sentData)
	if err != nil {
		t.Fatalf("Failed to send data: %v", err)
	}
	if n != dataSize {
		t.Fatalf("Sent %d bytes, expected %d", n, dataSize)
	}

	// 接收回传的数据
	receivedData := make([]byte, dataSize)
	_, err = io.ReadFull(conn, receivedData)
	if err != nil {
		t.Fatalf("Failed to read echoed data: %v", err)
	}
	if !bytes.Equal(sentData, receivedData) {
		t.Fatalf("Data mismatch between sent and received")
	}

	// 等待服务器完成处理
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("Timed out waiting for server echo")
	}
}
