package filestream

import (
    . "ml"
    . "ml/strings"
    "os"
    "io"
    "math"
    "unsafe"
    "reflect"
    "encoding/binary"
)

var (
    BigEndian       = &binary.BigEndian
    LittleEndian    = &binary.LittleEndian

    SEEK_SET        = os.SEEK_SET
    SEEK_CUR        = os.SEEK_CUR
    SEEK_END        = os.SEEK_END

    READ            = 1 << 0
    WRITE           = 1 << 1
    READWRITE       = READ | WRITE
)

const (
    END_OF_FILE     = -1
)

type File struct {
    file *os.File
    Endian binary.ByteOrder
}

func Open(name string) *File {
    return CreateFile(name, READ)
}

func Create(name string) *File {
    return CreateFile(name, READWRITE)
}

func CreateFile(name string, mode int) *File {
    flag := 0

    switch {
        case (mode & READWRITE) == READWRITE:
            flag = os.O_RDWR

        case (mode & READ) == READ:
            flag = os.O_RDONLY

        case (mode & WRITE) == WRITE:
            flag = os.O_WRONLY
    }

    f, err := os.OpenFile(name, flag, 0666)
    raiseFileError(err)

    return &File{f, LittleEndian}
}

func (self *File) Close() {
    self.file.Close()
}

func (self *File) Length() int64 {
    fi, err := self.file.Stat()
    raiseFileError(err)
    return fi.Size()
}

func (self *File) SetLength(length int64) {
    pos := self.Position()
    if pos > self.Length() {
        self.SetPosition(length)
        self.Write(byte(0))
    }

    err := self.file.Truncate(length)
    raiseFileError(err)
    if length < pos {
        pos = length
    }

    self.SetPosition(pos)
}

func (self *File) Remaining() int64 {
    return self.Length() - self.Position()
}

func (self *File) Position() int64 {
    return self.Seek(0, SEEK_CUR)
}

func (self *File) SetPosition(offset int64) {
    if offset == END_OF_FILE {
        self.Seek(0, SEEK_END)
        return
    }

    self.Seek(offset, SEEK_SET)
}

func (self *File) Seek(offset int64, whence int) int64 {
    pos, err := self.file.Seek(offset, whence)
    raiseFileError(err)
    return pos
}

func (self *File) Read(n int) []byte {
    buffer := [1024]byte{}
    data := []byte{}

    for n > 0 {
        bytesRead := If(n > len(buffer), len(buffer), n).(int)
        bytesRead, err := self.file.Read(buffer[:bytesRead])
        if err == io.EOF {
            data = append(data, buffer[:bytesRead]...)
            break
        }

        raiseFileError(err)

        data = append(data, buffer[:bytesRead]...)
        n -= bytesRead
    }

    return data
}

func (self *File) Write(args ...interface{}) int {
    buffer := []byte{}

    data := args[0]

    switch b := data.(type) {
        case bool:
            buffer = append(buffer, If(b, 1, 0).(byte))

        case int8, byte:
            buffer = append(buffer, b.(byte))

        case int16, uint16:
            self.Endian.PutUint16(buffer, b.(uint16))

        case int32, uint32:
            self.Endian.PutUint32(buffer, b.(uint32))

        case int64, uint64:
            self.Endian.PutUint64(buffer, b.(uint64))

        case float32:
            self.Endian.PutUint32(buffer, math.Float32bits(b))

        case float64:
            self.Endian.PutUint64(buffer, math.Float64bits(b))

        case []byte:
            buffer = b

        case String:
            codpage := CP_UTF8
            if len(args) > 1 {
                codpage = args[1].(Encoding)
            }

            buffer = b.Encode(codpage)

        case string:
            s := String(b)
            codpage := CP_UTF8
            if len(args) > 1 {
                codpage = args[1].(Encoding)
            }

            buffer = s.Encode(codpage)
    }

    n, err := self.file.Write(buffer)
    raiseFileError(err)
    return n
}

func (self *File) ReadAll() (data []byte) {
    length := self.Length()

    for length > 0 {
        read := self.Read(int(length))
        if len(read) == 0 {
            break
        }

        length -= int64(len(read))
        data = append(data, read...)
    }

    return
}

func (self *File) Flush() {
    raiseFileError(self.file.Sync())
}

func (self *File) IsEndOfFile() bool {
    return self.Position() >= self.Length()
}

func (self *File) ReadBoolean() bool {
    return self.ReadByte() != 0
}

func (self *File) ReadChar() int {
    return int(int8(self.Read(1)[0]))
}

func (self *File) ReadByte() uint {
    return uint(self.Read(1)[0])
}

func (self *File) ReadShort() int {
    return int(int16(self.Endian.Uint16(self.Read(2))))
}

func (self *File) ReadUShort() uint {
    return uint(self.Endian.Uint16(self.Read(2)))
}

func (self *File) ReadLong() int {
    return int(self.Endian.Uint32(self.Read(4)))
}

func (self *File) ReadULong() uint {
    return uint(self.Endian.Uint32(self.Read(4)))
}

func (self *File) ReadLong64() int64 {
    return int64(self.Endian.Uint64(self.Read(8)))
}

func (self *File) ReadULong64() uint64 {
    return self.Endian.Uint64(self.Read(8))
}

func (self *File) ReadFloat() float32 {
    return math.Float32frombits(uint32(self.ReadULong()))
}

func (self *File) ReadDouble() float64 {
    return math.Float64frombits(self.ReadULong64())
}

func (self *File) ReadMultiByte(encoding ...Encoding) String {
    bytes := []byte{}

    codepage := CP_UTF8

    switch len(encoding) {
        case 1:
            codepage = encoding[0]
    }

    for {
        ch := self.ReadByte()
        if ch == 0 {
            break
        }

        bytes = append(bytes, byte(ch))
    }

    return Decode(bytes, codepage)
}

func (self *File) ReadUTF16() String {
    bytes := []byte{}

    codepage := CP_UTF16_LE
    if self.Endian == BigEndian {
        codepage = CP_UTF16_BE
    }

    for {
        ch := self.ReadUShort()
        if ch == 0 {
            break
        }

        bytes = append(bytes, byte(ch), byte(ch >> 8))
    }

    return Decode(bytes, codepage)
}

func (self *File) ReadType(t interface{}) interface{} {
    typ := reflect.TypeOf(t)
    bytes:= self.Read(int(typ.Size()))
    return reflect.NewAt(typ, unsafe.Pointer(&bytes[0])).Elem().Interface()
}