package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var (
	mu      sync.Mutex
	clients map[net.Conn]bool
)

func main() {
	// 创建连接列表
	clients = make(map[net.Conn]bool)

	var addr string
	flag.StringVar(&addr, "addr", ":8111", "Address for listen")
	flag.Parse()

	// 创建TCP监听器
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	log.Printf("Start server at %s \n", addr)

	// 捕获终止信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		// 发送关闭消息给所有客户端
		mu.Lock()
		for conn := range clients {
			conn.Write([]byte("Server Shutdown\n"))
			conn.Close()
		}
		clients = make(map[net.Conn]bool) // 清空连接列表
		mu.Unlock()
		os.Exit(0)
	}()

	// 接受连接
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		fmt.Printf("新连接: %s\n", conn.RemoteAddr())
		// 将连接加入列表
		mu.Lock()
		clients[conn] = true
		mu.Unlock()
		// 处理连接
		go handleConnection2(conn)
	}
}

func handleConnection2(conn net.Conn) {
	// 创建Ticker，每5秒发送一次消息
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	// 创建一个通道来接收读取的行
	readChan := make(chan string)
	// 启动读取数据的goroutine
	go func() {
		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				readChan <- ""
				return
			}
			readChan <- line
		}
	}()
	defer func() {
		// 连接关闭时从列表中移除
		mu.Lock()
		delete(clients, conn)
		mu.Unlock()
		conn.Close()
	}()
	for {
		select {
		case <-ticker.C:
			// 获取当前时间并格式化
			currentTime := time.Now().Format("2006-01-02 15:04:05")
			message := "Hello World " + currentTime + "\n"
			_, err := conn.Write([]byte(message))
			if err != nil {
				// 发送失败，移除连接
				return
			}
		case line := <-readChan:
			if line == "" {
				// 读取错误，关闭连接
				fmt.Printf("连接 %s 已关闭\n", conn.RemoteAddr())
				conn.Write([]byte("Bye\n"))
				fmt.Printf("发送 Bye 给客户端 %s\n", conn.RemoteAddr())
				conn.Close()
				return
			}
			// 打印来自客户端的输入
			fmt.Printf("来自客户端 %s 的输入:长度 %d 内容 %s\n", conn.RemoteAddr(), len(line), line)
			// 检查是否是结束信号
			if line == "QUIT\r\n" || line == "EXIT\r\n" {
				conn.Write([]byte("Bye\n"))
				fmt.Printf("发送 Bye 给客户端 %s\n", conn.RemoteAddr())
				conn.Close()
				return
			}
			// 处理其他数据...
		}
	}
}
