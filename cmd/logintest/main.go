package main

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"time"

	"niaohao/server/internal/packet"
)

// 联调：104 → 108(可选) → 105 → 1001 → 收取 2001/2003/2004
func main() {
	loginAddr := "127.0.0.1:22973"
	email := "test@niao.local"
	pass := "123456"
	if len(os.Args) > 1 {
		loginAddr = os.Args[1]
	}
	if len(os.Args) > 2 {
		email = os.Args[2]
	}

	sum := md5.Sum([]byte(pass))
	passMD5 := hex.EncodeToString(sum[:])

	conn, err := net.DialTimeout("tcp", loginAddr, 3*time.Second)
	must(err)
	defer conn.Close()

	body := make([]byte, 96)
	copy(body[0:64], []byte(email))
	copy(body[64:96], []byte(passMD5))
	send(conn, 104, 0, body)
	cmd, uid, result, resp := recv(conn)
	fmt.Printf("104 => cmd=%d uid=%d result=%d body=%d\n", cmd, uid, result, len(resp))
	if result != 0 || len(resp) < 20 {
		os.Exit(1)
	}
	session := append([]byte{}, resp[0:16]...)
	roleCreate := binary.BigEndian.Uint32(resp[16:20])
	fmt.Printf("session=%s roleCreate=%d\n", hex.EncodeToString(session), roleCreate)

	if roleCreate == 0 {
		b108 := make([]byte, 24)
		binary.BigEndian.PutUint32(b108[0:4], uid)
		copy(b108[4:20], []byte("测试员"))
		binary.BigEndian.PutUint32(b108[20:24], 1)
		send(conn, 108, uid, b108)
		cmd, uid, result, resp = recv(conn)
		fmt.Printf("108 => cmd=%d uid=%d result=%d body=%d\n", cmd, uid, result, len(resp))
		if result != 0 || len(resp) < 16 {
			os.Exit(1)
		}
		session = append([]byte{}, resp[0:16]...)
	}

	send(conn, 105, uid, nil)
	cmd, uid, result, resp = recv(conn)
	fmt.Printf("105 => cmd=%d uid=%d result=%d body=%d\n", cmd, uid, result, len(resp))
	if result != 0 || len(resp) < 12+30 {
		os.Exit(1)
	}
	ip := string(bytesTrim(resp[20:36]))
	port := binary.BigEndian.Uint16(resp[36:38])
	fmt.Printf("game %s:%d\n", ip, port)

	gconn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), 3*time.Second)
	must(err)
	defer gconn.Close()
	send(gconn, 1001, uid, session)

	got1001, got2001, got2003, got2004 := false, false, false, false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_ = gconn.SetReadDeadline(time.Now().Add(800 * time.Millisecond))
		cmd, uid, result, resp = recv(gconn)
		fmt.Printf("game <= cmd=%d uid=%d result=%d body=%d\n", cmd, uid, result, len(resp))
		if result != 0 {
			fmt.Printf("FAIL result=%d\n", result)
			os.Exit(1)
		}
		switch cmd {
		case 1001:
			got1001 = true
		case 2001:
			got2001 = true
			if len(resp) < 144 {
				fmt.Printf("FAIL 2001 peopleInfo too short: %d\n", len(resp))
				os.Exit(1)
			}
			pid := binary.BigEndian.Uint32(resp[4:8])
			if pid != uid {
				fmt.Printf("FAIL 2001 userID=%d want=%d\n", pid, uid)
				os.Exit(1)
			}
		case 2003:
			got2003 = true
			if len(resp) < 4 {
				os.Exit(1)
			}
		case 2004:
			got2004 = true
			if len(resp) != 36 {
				fmt.Printf("FAIL 2004 want 36 got %d\n", len(resp))
				os.Exit(1)
			}
		}
		if got1001 && got2001 && got2003 && got2004 {
			break
		}
	}
	if !got1001 || !got2001 || !got2003 || !got2004 {
		fmt.Printf("FAIL missing packets 1001=%v 2001=%v 2003=%v 2004=%v\n", got1001, got2001, got2003, got2004)
		os.Exit(1)
	}
	fmt.Println("LOGIN + ENTER MAP FLOW OK")
}

func send(conn net.Conn, cmd int32, uid uint32, body []byte) {
	_, err := conn.Write(packet.BuildResponse(cmd, uid, 0, body))
	must(err)
}

func recv(conn net.Conn) (cmd int32, uid uint32, result int32, body []byte) {
	hdr := make([]byte, 17)
	must(readFull(conn, hdr))
	pkgLen := int(binary.BigEndian.Uint32(hdr[0:4]))
	cmd = int32(binary.BigEndian.Uint32(hdr[5:9]))
	uid = binary.BigEndian.Uint32(hdr[9:13])
	result = int32(binary.BigEndian.Uint32(hdr[13:17]))
	body = make([]byte, pkgLen-17)
	if len(body) > 0 {
		must(readFull(conn, body))
	}
	return
}

func readFull(conn net.Conn, b []byte) error {
	off := 0
	for off < len(b) {
		n, err := conn.Read(b[off:])
		if err != nil {
			return err
		}
		off += n
	}
	return nil
}

func bytesTrim(b []byte) []byte {
	i := len(b)
	for i > 0 && b[i-1] == 0 {
		i--
	}
	return b[:i]
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
