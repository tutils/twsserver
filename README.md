# twsserver
websocket server as http.Handler

## Example
```go
h := &handler{}
mux := twsserver.NewServeMux()
mux.HandleFunc(CmdPing, h.Ping)
mux.HandleFunc(CmdLogin, h.Login)
mux.HandleFunc(CmdEnter, h.EnterChan)
mux.HandleFunc(CmdExit, h.ExitChan)
mux.HandleFunc(CmdSendToClient, h.SendToClient)
mux.HandleFunc(CmdSendToUser, h.SendToUser)
mux.HandleFunc(CmdSendToChan, h.SendToChan)
mux.HandleFunc(CmdRecvData, h.RecvData)
ws := twsserver.Server(
    twsserver.WithServeMux(mux),
    twsserver.WithRecvTimeout(RecvTimeout),
    twsserver.WithUpgradeHandler(h.OpUpgrade),
    twsserver.WithOpenHandler(h.OnOpen),
    twsserver.WithCloseHandler(h.OnClose),
)
ws.StartWritePumps(runtime.NumCPU())

// 注册web服务处理器
http.Handle("/stream", ws)
http.Handle("/", http.FileServer(http.Dir("html")))
http.ListenAndServe(":http", nil)
```
