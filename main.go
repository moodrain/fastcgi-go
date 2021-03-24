package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
)

const HEAD_LEN = 8

// fcgi_request_type 请求类型
const FCGI_BEGIN_REQUEST      = 1
const FCGI_ABORT_REQUEST      = 2
const FCGI_END_REQUEST        = 3
const FCGI_PARAMS             = 4
const FCGI_STDIN              = 5
const FCGI_STDOUT             = 6
const FCGI_STDERR             = 7
const FCGI_DATA               = 8
const FCGI_GET_VALUES         = 9
const FCGI_GET_VALUES_RESULT  = 10
const FCGI_UNKNOWN_TYPE       = 11

// fcgi_role 服务器希望 fastcgi 充当的角色
const FCGI_RESPONDER   = 1
const FCGI_AUTHORIZER  = 2
const FCGI_FILTER      = 3

// 记录 头 结构
type FcgiHeader struct {
	Version byte
	Type byte
	RequestId uint16
	ContentLength uint16
	//RequestIdB1 byte
	//RequestIdB0 byte
	//ContentLengthB1 byte
	//ContentLengthB0 byte
	PaddingLength byte
	Reserved byte
}

// 记录 请求开始 结构
type FcgiBeginRequestBody struct {
	Role uint16
	// RoleB1 byte
	// RoleB0 byte
	Flags byte
	Reserved [5]byte
}

// 记录 请求结束 结构
type FcgiEndRequestBody struct {
	AppStatus uint32
	// AppStatusB3 byte
	// AppStatusB2 byte
	// AppStatusB1 byte
	// AppStatusB0 byte
	ProtocolStatus byte
	Reserved [3]byte
}

// 请求结束记录 的 协议状态
const FCGI_REQUEST_COMPLETE = 0
const FCGI_CANT_MPX_CONN    = 1
const FCGI_OVERLOADED       = 2
const FCGI_UNKNOWN_ROLE     = 3

func HeaderTypeName(code byte) string {
	switch code {
	case 1: return "FCGI_BEGIN_REQUEST"
	case 4: return "FCGI_PARAMS"
	case 5: return "FCGI_STDIN"
	}
	return ""
}

func ReadHead(buff []byte) FcgiHeader {
	head := FcgiHeader{
		Version:       buff[0],
		Type:          buff[1],
		RequestId:     uint16(buff[2]) << 8 | uint16(buff[3]),
		ContentLength: uint16(buff[4]) << 8 | uint16(buff[5]),
		PaddingLength: buff[6],
		Reserved:      buff[7],
	}
	if head.ContentLength > 0 {
		fmt.Println("----- Read Header -----")
		fmt.Println("ID: ", head.RequestId)
		fmt.Println("Type: ", HeaderTypeName(head.Type))
		fmt.Println("Length: ", head.ContentLength)
	}
	return head
}

func ReadConn(conn net.Conn, header FcgiHeader) []byte {
	length := header.ContentLength + uint16(header.PaddingLength)
	buff := make([]byte, length)
	size, err := conn.Read(buff)
	buff = buff[:header.ContentLength]
	if err != nil || size != int(length) {
		log.Fatal("Read " + HeaderTypeName(header.Type) + " Error")
	}
	if header.ContentLength > 0 {
		fmt.Println("----- " + HeaderTypeName(header.Type) + " -----")
	}
	return buff
}

func ReadBeginRequest(header FcgiHeader, conn net.Conn) {
	buff := ReadConn(conn, header)
	fmt.Println("Role", uint16(buff[0]) << 8 | uint16(buff[1]))
	fmt.Println("Flag", buff[2])
}

func ReadParamsRequest(header FcgiHeader, conn net.Conn) map[string]string {
	buff := ReadConn(conn, header)
	pos := 0
	kvMap := make(map[string]string)
	for {
		if pos >= len(buff) {
			break
		}
		keyLen := int(buff[pos])
		if keyLen > 127 {
			keyLen = int(buff[pos] << 24) | int(buff[pos+1] << 16) | int(buff[pos+2] << 8) | int(buff[pos+3])
			pos += 3
		}
		pos++
		valLen := int(buff[pos])
		if valLen > 127 {
			valLen = int(buff[pos] << 24) | int(buff[pos+1] << 16) | int(buff[pos+2] << 8) | int(buff[pos+3])
			pos += 3
		}
		pos++
		key := string(buff[pos:pos+keyLen])
		pos += keyLen
		value := string(buff[pos:pos+valLen])
		kvMap[key] = value
		pos += valLen
	}
	for key, val := range kvMap {
		fmt.Println(key + ": " + val)
	}
	return kvMap
}

func ReadStdinRequest(header FcgiHeader, conn net.Conn) string {
	if header.ContentLength == 0 {
		return ""
	}
	buff := ReadConn(conn, header)
	fmt.Println(string(buff))
	return string(buff)
}

func ExecPhp(env map[string]string, data string) string {
	for key, val := range env {
		_ = os.Setenv(key, val)
	}
	cmd := exec.Command("php", env["SCRIPT_FILENAME"], "--post=" + data)
	rs, err := cmd.Output()
	if err != nil {
		log.Fatal("Exec PHP Error")
	}
	for key, _ := range env {
		_ = os.Unsetenv(key)
	}
	return string(rs)
}

func SendResponse(id uint16, content string, conn net.Conn) {
	buff := make([]byte, HEAD_LEN)
	htmlHead := "Content-Type: text/html\r\n\r\n"
	htmlBody := content
	contentLen := len(htmlHead) + len(htmlBody)
	padLen := byte((8 - (contentLen % 8)) % 8)
	pad := make([]byte, padLen)

	buff[0] = 1
	buff[1] = FCGI_STDOUT
	buff[2] = byte(id >> 8)
	buff[3] = byte(id)
	buff[4] = byte(contentLen >> 8)
	buff[5] = byte(contentLen)
	buff[6] = padLen
	buff[7] = 0
	_, err := conn.Write(buff)
	_, err = conn.Write([]byte(htmlHead))
	_, err = conn.Write([]byte(htmlBody))
	_, err = conn.Write(pad)

	buff = make([]byte, HEAD_LEN)
	buff[0] = 1
	buff[1] = FCGI_STDOUT
	_, err = conn.Write(buff)

	buff = make([]byte, HEAD_LEN)
	buff[0] = 1
	buff[1] = FCGI_END_REQUEST
	buff[5] = HEAD_LEN
	_, err = conn.Write(buff)

	buff = make([]byte, HEAD_LEN)
	_, err = conn.Write(buff)

	if err != nil {
		log.Fatal("Send Response Error")
	}
	fmt.Println("----- Send Response -----")
	fmt.Println("Content", htmlHead + htmlBody)
}

func main() {
	server, _ := net.Listen("tcp", "127.0.0.1:9001")
	fmt.Println("Start Listen Request")
	conn, _ := server.Accept()
	var id uint16
	env := make(map[string]string)
	var data string
	process := make(map[string]bool)
	for {
		buff := make([]byte, HEAD_LEN)
		if _, err := conn.Read(buff); err != nil {
			_ = conn.Close()
			if conn, err = server.Accept(); err != nil {
				log.Fatal("Network Error")
			} else {
				if _, err = conn.Read(buff); err != nil {
					log.Fatal("Network Error")
				}
			}
		}
		head := ReadHead(buff)
		if id == 0 && head.RequestId != 0 {
			id = head.RequestId
		}
		if id != head.RequestId {
			continue
		}
		switch head.Type {
		case FCGI_BEGIN_REQUEST: ReadBeginRequest(head, conn)
		case FCGI_PARAMS:
			if newEnv := ReadParamsRequest(head, conn); len(newEnv) > 0 {
				env = newEnv
			}
			process["params"] = true
		case FCGI_STDIN:
			if newData := ReadStdinRequest(head, conn); len(newData) > 0 {
				data = newData
			}
			process["stdin"] = true
		default:
			log.Fatal("Unknown Request Type", head.Type)
		}
		if process["params"] && process["stdin"] {
			rs := ExecPhp(env, data)
			SendResponse(id, rs, conn)
			id = 0
			process["params"] = false
			process["stdin"] = false
		}
	}
}
