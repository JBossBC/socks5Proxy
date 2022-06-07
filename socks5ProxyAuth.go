package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
)

type authServer struct {
	user     string
	password string
}

var server authServer = authServer{
	user:     "1577002722",
	password: "jiang19780809",
}

//目前支持的验证方式一共有：
//
//X’00’ NO AUTHENTICATION REQUIRED（不需要验证）
//X’01’ GSSAPI
//X’02’ USERNAME/PASSWORD（用户名密码）
//X’03’ to X’7F’ IANA ASSIGNED
//X’80’ to X’FE’ RESERVED FOR PRIVATE METHODS
//X’FF’ NO ACCEPTABLE METHODS（都不支持，没法连接了）

func main() {
	Listen, err := net.Listen("tcp", "127.0.0.1:8000")
	if err != nil {
		fmt.Println(err.Error())
	}
	for {
		conn, acceptErr := Listen.Accept()
		if acceptErr != nil {
			fmt.Println(err.Error())
		}
		go process(conn)

	}
}
func process(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	err := ack(reader, conn)
	err = auth(reader, conn)
	if err != nil {
		log.Printf("client %v auth failed %v", conn.RemoteAddr(), err)
		return
	}
	err = connect(reader, conn)
	if err != nil {
		log.Printf("client %v auth failed:%v", conn.RemoteAddr(), err)
		return
	}

}

func auth(reader *bufio.Reader, conn net.Conn) (err error) {
	ver, _ := reader.ReadByte()
	if ver != 0x01 {
		return fmt.Errorf("not supported ver:%v", ver)
	}
	userLength, _ := reader.ReadByte()
	user := make([]byte, userLength)
	io.ReadFull(reader, user)
	passwordLength, _ := reader.ReadByte()
	password := make([]byte, passwordLength)
	io.ReadFull(reader, password)
	if string(user) != server.user && string(password) != server.password {
		return fmt.Errorf("auth failed,the password or user is error")
	}
	_, err = conn.Write([]byte{socks5Ver, 0x00})
	return nil
}

const socks5Ver = 0x05
const cmdBind = 0x01
const atypIPV4 = 0x01
const atypeHOST = 0x03
const atypeIPV6 = 0x04

func ack(reader *bufio.Reader, conn net.Conn) (err error) {
	ver, err := reader.ReadByte()
	if err != nil {
		return fmt.Errorf("read ver failed:%w", err)
	}
	if ver != socks5Ver {
		return fmt.Errorf("not supported ver:%v", ver)

	}
	methodSize, err := reader.ReadByte()
	if err != nil {
		return fmt.Errorf("read methodSize failed:%w", err)
	}
	method := make([]byte, methodSize)
	// fmt.Println("ver", ver, "method", method)
	_, err = io.ReadFull(reader, method)
	if err != nil {
		return fmt.Errorf("read method failed:%w", err)

	}
	_, err = conn.Write([]byte{socks5Ver, 0x02})
	if err != nil {
		return fmt.Errorf("write failed:%w", err)
	}
	return nil
}
func connect(reader *bufio.Reader, conn net.Conn) (err error) {
	// +----+-----+-------+------+----------+----------+
	// |VER | REP |  RSV  | ATYP | BND.ADDR | BND.PORT |
	// +----+-----+-------+------+----------+----------+
	// | 1  |  1  | X'00' |  1   | Variable |    2     |
	// +----+-----+-------+------+----------+----------+
	// VER socks版本，这里为0x05
	// REP Relay field,内容取值如下 X’00’ succeeded
	// RSV 保留字段
	// ATYPE 地址类型
	// BND.ADDR 服务绑定的地址
	// BND.PORT 服务绑定的端口DST.PORT
	buf := make([]byte, 4)
	_, err = io.ReadFull(reader, buf)
	if err != nil {
		return fmt.Errorf("read header failed:%w", err)
	}
	ver, cmd, atyp := buf[0], buf[1], buf[3]
	if ver != socks5Ver {
		return fmt.Errorf("not supported ver:%v", ver)
	}
	if cmd != cmdBind {
		return fmt.Errorf("not supported cmd:%v", cmd)
	}
	addr := ""
	switch atyp {
	case atypIPV4:
		_, err = io.ReadFull(reader, buf)
		if err != nil {
			return fmt.Errorf("read atyp failed:%w", err)
		}
		addr = fmt.Sprintf("%d.%d.%d.%d", buf[0], buf[1], buf[2], buf[3])
	case atypeHOST:
		hostSize, err := reader.ReadByte()
		if err != nil {
			return fmt.Errorf("read hostSize failed:%w", err)
		}
		host := make([]byte, hostSize)
		_, err = io.ReadFull(reader, host)
		if err != nil {
			return fmt.Errorf("read host failed:%w", err)
		}
		addr = string(host)
	case atypeIPV6:
		return errors.New("IPv6: no supported yet")
	default:
		return errors.New("invalid atyp")

	}
	_, err = io.ReadFull(reader, buf[:2])
	if err != nil {
		return fmt.Errorf("read port failed:%w", err)
	}
	port := binary.BigEndian.Uint16(buf[:2])

	dest, err := net.Dial("tcp", fmt.Sprintf("%v:%v", addr, port))
	if err != nil {
		return fmt.Errorf("dial dst failed:%w", err)
	}
	defer dest.Close()
	log.Println("dial", addr, port)
	_, err = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_, _ = io.Copy(dest, reader)
		cancel()
	}()
	go func() {
		_, _ = io.Copy(conn, dest)
		cancel()
	}()

	<-ctx.Done()
	return nil
}
