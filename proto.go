package twsserver

import "github.com/mitchellh/mapstructure"

func DecodeData(payload interface{}, data interface{}) error {
	return mapstructure.Decode(payload, data)
}

func EncodeData(data interface{}) interface{} {
	return data
}

type Request interface {
	Command() string
	Sequence() int64
	DecodeData(data interface{}) error
	Conn() *Conn
}

type Response interface {
	EncodeData(data interface{}, code int32, msg string)
}

type RequestData struct {
	Cmd   string      `json:"cmd"`
	Seq   int64       `json:"seq"`
	Immed bool        `json:"immed,omitempty"`
	Data  interface{} `json:"data,omitempty"`
}

type ResponseData struct {
	Cmd  string      `json:"cmd"`
	Seq  int64       `json:"seq"`
	Code int32       `json:"code"`
	Msg  string      `json:"msg,omitempty"`
	Data interface{} `json:"data,omitempty"`
}

type request struct {
	data *RequestData
	conn *Conn
}

func (req *request) Conn() *Conn {
	return req.conn
}

func (req *request) Command() string {
	return req.data.Cmd
}

func (req *request) Sequence() int64 {
	return req.data.Seq
}

func (req *request) DecodeData(data interface{}) error {
	return mapstructure.Decode(req.data.Data, data)
}

type response struct {
	data *ResponseData
}

func (rsp *response) EncodeData(data interface{}, code int32, msg string) {
	rsp.data.Code = code
	rsp.data.Msg = msg
	rsp.data.Data = data
}
