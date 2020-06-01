package twsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	leftSB  = &bytesBuffer{b: []byte("[")}
	rightSB = &bytesBuffer{b: []byte("]")}
	comma   = &bytesBuffer{b: []byte(",")}
)

type ConnGroup struct {
	conns []interface{}
}

func (cg *ConnGroup) Conns(output *[]*Conn) {
	if size := len(cg.conns); cap(*output) < size {
		*output = make([]*Conn, size)
	} else {
		*output = (*output)[:size]
	}
	for i, o := range cg.conns {
		(*output)[i] = o.(*Conn)
	}
}

func (cg *ConnGroup) Write(cmd string, seq int64, data interface{}, code int32, msg string, immed bool) {
	if len(cg.conns) == 0 {
		return
	}

	rspData := &ResponseData{
		Cmd:  cmd,
		Seq:  seq,
		Code: code,
		Msg:  msg,
		Data: EncodeData(data),
	}

	//log.Println("conngroup begin to encode")
	buf := newMultiRefBuffer(int32(len(cg.conns)))
	if err := json.NewEncoder(buf).Encode(rspData); err != nil {
		return
	}

	//log.Println("conngroup begin to write")
	for _, c := range cg.conns {
		conn := c.(*Conn)
		conn.write(buf, immed)
	}
	//log.Println("conngroup write complete")
}

func NewConnGroup(conns []interface{}) *ConnGroup {
	cg := &ConnGroup{
		conns,
	}
	return cg
}

type Conn struct {
	svc         *server
	conn        *websocket.Conn
	ctx         context.Context
	writeq      []multiRefBytesBuffer
	mu          sync.Mutex
	closed      bool
	sendMu      sync.Mutex
	recvTimeout time.Duration
	sendTimeout time.Duration
}

func (c *Conn) ContextValue(key interface{}) interface{} {
	return c.ctx.Value(key)
}

func (c *Conn) AddContextValue(key, value interface{}) {
	c.ctx = context.WithValue(c.ctx, key, value)
}

func (c *Conn) Close() {
	_ = c.conn.Close()
}

func (c *Conn) shutdown() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}

	for _, bs := range c.writeq {
		bs.Unref()
	}
	c.writeq = c.writeq[:0]

	_ = c.conn.Close()
	//c.writeTimer.Reset(0)
	c.closed = true
	c.mu.Unlock()

	if c.svc.opt.closeHandler != nil {
		c.svc.opt.closeHandler(c)
	}
}

func (c *Conn) Write(cmd string, seq int64, data interface{}, code int32, msg string, immed bool) {
	rspData := &ResponseData{
		Cmd:  cmd,
		Seq:  seq,
		Code: code,
		Msg:  msg,
		Data: EncodeData(data),
	}

	buf := newMultiRefBuffer(1)
	if err := json.NewEncoder(buf).Encode(rspData); err != nil {
		return
	}
	c.write(buf, immed)
}

func (c *Conn) write(jsonStr multiRefBytesBuffer, immed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if immed {
		var buf bytes.Buffer
		buf.Write(leftSB.Bytes())
		buf.Write(jsonStr.Bytes())
		buf.Write(rightSB.Bytes())
		c.send(buf.Bytes())
		jsonStr.Unref()
		return
	}

	if len(c.writeq) == 0 {
		c.writeq = append(c.writeq, leftSB, jsonStr)
		c.svc.ready <- c
	} else {
		c.writeq = append(c.writeq, comma, jsonStr)
	}
}

func (c *Conn) swap(writer io.Writer) (closed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return true
	}

	if len(c.writeq) == 0 {
		return false
	}

	for _, bs := range c.writeq {
		writer.Write(bs.Bytes())
		bs.Unref()
	}
	writer.Write(rightSB.Bytes())
	c.writeq = c.writeq[:0]
	return false
}

func (c *Conn) send(data []byte) error {
	//log.Printf("%09d sent Response: %s", time.Now().UnixNano()%int64(time.Second), data)
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.sendTimeout > 0 {
		c.conn.SetWriteDeadline(time.Now().Add(c.sendTimeout))
	}
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

func (c *Conn) ping() error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.sendTimeout > 0 {
		c.conn.SetWriteDeadline(time.Now().Add(c.sendTimeout))
	}
	return c.conn.WriteMessage(websocket.PingMessage, nil)
}

func (c *Conn) run() error {
	//log.Println("run start")
	defer func() {
		c.shutdown()
		//log.Println("run complete")
	}()

	if c.svc.opt.openHandler != nil {
		if err := c.svc.opt.openHandler(c); err != nil {
			return err
		}
	}

	buf := newMultiRefBuffer(1)
	defer buf.Unref()
	en := json.NewEncoder(buf)
	for {
		var reqDatas []*RequestData
		if c.recvTimeout > 0 {
			c.conn.SetReadDeadline(time.Now().Add(c.recvTimeout))
		}

		if err := c.conn.ReadJSON(&reqDatas); err != nil {
			if !websocket.IsCloseError(err) || websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				// 意外的错误
				log.Println(err)
			} else {
				//log.Println(err)
			}
			return err
		}

		for _, reqData := range reqDatas {
			if reqData == nil {
				err := errors.New("nil request")
				log.Println(err)
				return err
			}
			//log.Printf("%09d received request: %v", time.Now().UnixNano()%int64(time.Second), reqData)

			handler := c.svc.opt.mux.Handler(reqData.Cmd)
			req := &request{
				data: reqData,
				conn: c,
			}
			rsp := &response{
				data: &ResponseData{
					Cmd: reqData.Cmd,
					Seq: reqData.Seq,
				},
			}
			if err := handler(req, rsp); err != nil {
				// 发生错误关闭连接
				log.Println(err)
				return err
			}

			buf.Reset()
			if err := en.Encode(rsp.data); err != nil {
				log.Println(err)
				return err
			}
			buf.Ref(1)
			c.write(buf, req.data.Immed)
		}
	}
}

func newConn(svc *server, conn *websocket.Conn, recvWait time.Duration, sendWait time.Duration, tcpKeepAlive bool) *Conn {
	if tcpKeepAlive {
		if sock, ok := conn.UnderlyingConn().(*net.TCPConn); ok {
			sock.SetKeepAlive(true)
			sock.SetKeepAlivePeriod(recvWait)
		}
	}

	c := &Conn{
		svc:         svc,
		conn:        conn,
		ctx:         context.Background(),
		closed:      false,
		recvTimeout: recvWait,
		sendTimeout: sendWait,
	}
	return c
}

type multiRefBytesBuffer interface {
	Bytes() []byte
	Ref(num int32)
	Unref()
}

type multiRefBuffer struct {
	bytes.Buffer
	ref int32 //accessed atomically
}

func (b *multiRefBuffer) Ref(num int32) {
	atomic.AddInt32(&b.ref, num)
}

func (b *multiRefBuffer) Unref() {
	ref := atomic.AddInt32(&b.ref, -1)
	if ref <= 0 {
		if ref < 0 {
			panic("ref < 0")
		}
		b.Reset()
		jsonStrPoolBuffer.Put(b)
		//debugNum := atomic.AddInt32(&debugNumOfBuffers, -1)
		//log.Printlnf("cur debugNumOfBuffers: %d", debugNum)
	}
}

//var debugNumOfBuffers int32
var jsonStrPoolBuffer = &sync.Pool{New: func() interface{} { return &multiRefBuffer{} }}

func newMultiRefBuffer(ref int32) *multiRefBuffer {
	//atomic.AddInt32(&debugNumOfBuffers, 1)
	buf := jsonStrPoolBuffer.Get().(*multiRefBuffer)
	buf.Ref(ref)
	return buf
}

type bytesBuffer struct {
	b []byte
}

func (b *bytesBuffer) Bytes() []byte {
	return b.b
}

func (b *bytesBuffer) Ref(num int32) {
}

func (b *bytesBuffer) Unref() {
}
