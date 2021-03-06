package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	rpc "github.com/youtube/vitess/go/rpcplus"
	"github.com/youtube/vitess/go/rpcwrap/bsonrpc"
)

type Args struct {
	A, B int
}

type Quotient struct {
	Quo, Rem int
}

type Arith int

func (t *Arith) Multiply(args *Args, reply *int) error {
	*reply = args.A * args.B
	return nil
}

func (t *Arith) Divide(args *Args, quo *Quotient) error {
	if args.B == 0 {
		return errors.New("divide by zero")
	}
	quo.Quo = args.A / args.B
	quo.Rem = args.A % args.B
	return nil
}

type SleepArgs struct {
	Duration int
}

func (t *Arith) Sleep(args *SleepArgs, ret *bool) error {
	time.Sleep(time.Duration(args.Duration) * time.Millisecond)
	*ret = true
	return nil
}

type StreamingArgs struct {
	A     int
	Count int
	// next two values have to be between 0 and Count-2 to trigger anything
	ErrorAt   int // will trigger an error at the given spot,
	BadTypeAt int // will send the wrong type in sendReply
}

type StreamingReply struct {
	C     int
	Index int
}

func (t *Arith) Thrive(args StreamingArgs, sendReply func(reply interface{}) error) error {

	for i := 0; i < args.Count; i++ {
		if i == args.ErrorAt {
			return errors.New("Triggered error in middle")
		}
		if i == args.BadTypeAt {
			// send args instead of response
			sr := new(StreamingArgs)
			err := sendReply(sr)
			if err != nil {
				return err
			}
		}
		sr := &StreamingReply{C: args.A, Index: i}
		err := sendReply(sr)
		if err != nil {
			return err
		}
	}

	return nil
}

type IncrementArgs struct {
	Num uint64
}

func (t *Arith) Increment(args *IncrementArgs, reply *uint64) error {
	*reply = args.Num + 1
	return nil
}

type rpcHandler struct{}

func (h *rpcHandler) ServeHTTP(c http.ResponseWriter, req *http.Request) {
	if req.Method != "CONNECT" {
		c.Header().Set("Content-Type", "text/plain; charset=utf-8")
		c.WriteHeader(http.StatusMethodNotAllowed)
		io.WriteString(c, "405 must CONNECT\n")
		return
	}
	conn, _, err := c.(http.Hijacker).Hijack()
	if err != nil {
		log.Print("rpc hijacking ", req.RemoteAddr, ": ", err.Error())
		return
	}
	io.WriteString(conn, "HTTP/1.0 200 Connected to Go RPC\n\n")
	codec := bsonrpc.NewServerCodec(conn)
	rpc.ServeCodec(codec)
}

var l net.Listener

type shutdownHandler struct{}

func (h *shutdownHandler) ServeHTTP(c http.ResponseWriter, req *http.Request) {
	l.Close()
	c.Write([]byte("shutting down"))
}

func main() {
	port := flag.Int("port", 9279, "server port")
	flag.Parse()
	arith := new(Arith)
	rpc.Register(arith)

	http.Handle("/_bson_rpc_", &rpcHandler{})
	http.Handle("/shutdown", &shutdownHandler{})
	addr := fmt.Sprintf("localhost:%d", *port)
	var e error
	l, e = net.Listen("tcp", addr)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	log.Printf("arithserver running on %s", addr)
	http.Serve(l, nil)
}
