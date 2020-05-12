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
	"time"
)

type Writer interface {
	Write(cmd string, seq int64, data interface{}, code int32, msg string, immed bool)
}

type Client interface {
	Writer
	ContextValue(key interface{}) interface{}
	AddContextValue(key, value interface{})
	Close()
}

type ClientGroup interface {
	Writer
	Clients(output *[]Client)
}

var (
	leftSB  = []byte("[")
	rightSB = []byte("]")
	comma   = []byte(",")
)

type clientGroup struct {
	clients []interface{}
}

func (cg *clientGroup) Clients(output *[]Client) {
	if size := len(cg.clients); cap(*output) < size {
		*output = make([]Client, size)
	} else {
		*output = (*output)[:size]
	}
	for i, o := range cg.clients {
		(*output)[i] = o.(Client)
	}
}

func (cg *clientGroup) Write(cmd string, seq int64, data interface{}, code int32, msg string, immed bool) {
	if len(cg.clients) == 0 {
		return
	}

	rspData := &ResponseData{
		Cmd:  cmd,
		Seq:  seq,
		Code: code,
		Msg:  msg,
		Data: EncodeData(data),
	}

	log.Println("clientgroup begin to encode")
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(rspData); err != nil {
		return
	}

	log.Println("clientgroup begin to write")
	for _, c := range cg.clients {
		cli := c.(*client)
		cli.write(buf.Bytes(), immed)
	}
	log.Println("clientgroup write complete")
}

func NewClientGroup(clients []interface{}) ClientGroup {
	cg := &clientGroup{
		clients,
	}
	return cg
}

type client struct {
	svc         *server
	conn        *websocket.Conn
	ctx         context.Context
	writeq      [][]byte
	mu          sync.Mutex
	closed      bool
	sendMu      sync.Mutex
	immedWriter bytes.Buffer
	recvTimeout time.Duration
	sendTimeout time.Duration
}

func (c *client) ContextValue(key interface{}) interface{} {
	return c.ctx.Value(key)
}

func (c *client) AddContextValue(key, value interface{}) {
	c.ctx = context.WithValue(c.ctx, key, value)
}

func (c *client) Close() {
	_ = c.conn.Close()
}

func (c *client) shutdown() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}

	_ = c.conn.Close()
	//c.writeTimer.Reset(0)
	c.closed = true
	c.mu.Unlock()

	if c.svc.opt.closeHandler != nil {
		c.svc.opt.closeHandler(c)
	}
}

func (c *client) Write(cmd string, seq int64, data interface{}, code int32, msg string, immed bool) {
	rspData := &ResponseData{
		Cmd:  cmd,
		Seq:  seq,
		Code: code,
		Msg:  msg,
		Data: EncodeData(data),
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(rspData); err != nil {
		return
	}
	c.write(buf.Bytes(), immed)
}

func (c *client) write(json []byte, immed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if immed {
		c.immedWriter.Reset()
		c.immedWriter.WriteByte(leftSB[0])
		c.immedWriter.Write(json)
		c.immedWriter.WriteByte(rightSB[0])
		c.send(c.immedWriter.Bytes())
		return
	}

	if len(c.writeq) == 0 {
		c.writeq = append(c.writeq, leftSB, json)
		c.svc.ready <- c
	} else {
		c.writeq = append(c.writeq, comma, json)
	}
}

func (c *client) swap(writer io.Writer) (closed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return true
	}

	if len(c.writeq) == 0 {
		return false
	}

	for _, bs := range c.writeq {
		writer.Write(bs)
	}
	writer.Write(rightSB)
	c.writeq = c.writeq[:0]
	return false
}

func (c *client) send(data []byte) error {
	//log.Debugf("%09d sent Response: %s", time.Now().UnixNano()%int64(time.Second), data)
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.sendTimeout > 0 {
		c.conn.SetWriteDeadline(time.Now().Add(c.sendTimeout))
	}
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

func (c *client) ping() error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.sendTimeout > 0 {
		c.conn.SetWriteDeadline(time.Now().Add(c.sendTimeout))
	}
	return c.conn.WriteMessage(websocket.PingMessage, nil)
}

func (c *client) run() error {
	//log.Info("run start")
	defer func() {
		c.shutdown()
		//log.Info("run complete")
	}()

	if c.svc.opt.openHandler != nil {
		if err := c.svc.opt.openHandler(c); err != nil {
			return err
		}
	}

	var buf bytes.Buffer
	en := json.NewEncoder(&buf)
	for {
		var reqDatas []*RequestData
		if c.recvTimeout > 0 {
			c.conn.SetReadDeadline(time.Now().Add(c.recvTimeout))
		}

		if err := c.conn.ReadJSON(&reqDatas); err != nil {
			if !websocket.IsCloseError(err) || websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				// 意外的错误
				//log.Error(err)
			} else {
				//log.Debug(err)
			}
			return err
		}

		for _, reqData := range reqDatas {
			if reqData == nil {
				err := errors.New("nil request")
				//log.Error(err)
				return err
			}
			//log.Debugf("%09d received request: %v", time.Now().UnixNano()%int64(time.Second), reqData)

			handler := c.svc.opt.mux.Handler(reqData.Cmd)
			req := &request{
				data: reqData,
				cli:  c,
			}
			rsp := &response{
				data: &ResponseData{
					Cmd: reqData.Cmd,
					Seq: reqData.Seq,
				},
			}
			if err := handler(req, rsp); err != nil {
				// 发生错误关闭连接
				//log.Error(err)
				return err
			}

			buf.Reset()
			if err := en.Encode(rsp.data); err != nil {
				//log.Error(err)
				return err
			}
			c.write(buf.Bytes(), req.data.Immed)
		}
	}
}

func newClient(svc *server, conn *websocket.Conn, recvWait time.Duration, sendWait time.Duration, tcpKeepAlive bool) *client {
	if tcpKeepAlive {
		if sock, ok := conn.UnderlyingConn().(*net.TCPConn); ok {
			sock.SetKeepAlive(true)
			sock.SetKeepAlivePeriod(recvWait)
		}
	}

	c := &client{
		svc:         svc,
		conn:        conn,
		ctx:         context.Background(),
		closed:      false,
		recvTimeout: recvWait,
		sendTimeout: sendWait,
	}
	return c
}
