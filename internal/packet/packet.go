package packet

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const HeaderLen = 17
const VersionSV1 = 0x31 // ASCII '1'

// BuildResponse 构建 SV_1 响应。正常业务 result 必须为 0，否则客户端丢弃包体并可能流错位。
func BuildResponse(cmdID int32, userID uint32, result int32, body []byte) []byte {
	pkgLen := HeaderLen + len(body)
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, uint32(pkgLen))
	buf.WriteByte(VersionSV1)
	_ = binary.Write(buf, binary.BigEndian, uint32(cmdID))
	_ = binary.Write(buf, binary.BigEndian, userID)
	_ = binary.Write(buf, binary.BigEndian, uint32(result))
	buf.Write(body)
	return buf.Bytes()
}

func ParseHeader(data []byte) (pkgLen int, cmdID int32, userID uint32, seqOrResult int32, body []byte, err error) {
	if len(data) < HeaderLen {
		err = fmt.Errorf("packet too short: %d", len(data))
		return
	}
	pkgLen = int(binary.BigEndian.Uint32(data[0:4]))
	cmdID = int32(binary.BigEndian.Uint32(data[5:9]))
	userID = binary.BigEndian.Uint32(data[9:13])
	seqOrResult = int32(binary.BigEndian.Uint32(data[13:17]))
	if len(data) > HeaderLen {
		body = data[HeaderLen:]
	}
	return
}

func WriteU32(buf *bytes.Buffer, v uint32) {
	_ = binary.Write(buf, binary.BigEndian, v)
}

func WriteU16(buf *bytes.Buffer, v uint16) {
	_ = binary.Write(buf, binary.BigEndian, v)
}

func WriteFixedString(buf *bytes.Buffer, s string, n int) {
	b := []byte(s)
	if len(b) > n {
		b = b[:n]
	}
	buf.Write(b)
	if len(b) < n {
		buf.Write(make([]byte, n-len(b)))
	}
}

// WriteUTF 对齐 Flash ByteArray.writeUTF / IDataInput.readUTF：u16 长度 + UTF-8 字节。
func WriteUTF(buf *bytes.Buffer, s string) {
	b := []byte(s)
	if len(b) > 65535 {
		b = b[:65535]
	}
	WriteU16(buf, uint16(len(b)))
	buf.Write(b)
}

func ReadFixedString(data []byte) string {
	return string(bytes.TrimRight(data, "\x00"))
}
