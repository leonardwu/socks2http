package main

import (
	"flag"
	"golang.org/x/net/proxy"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
)

var (
	laddr = flag.String("laddr", "0.0.0.0:8180", "Address to listen on")
	raddr = flag.String("raddr", "socks5://127.0.0.1:1080", "Socks proxy address to connect to.")
)

type HttpProxyRoutineHandler struct {
	Dialer proxy.Dialer
}

func (h *HttpProxyRoutineHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hijack, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}

	port := r.URL.Port()
	if port == "" {
		port = "80"
	}
	socksConn, err := h.Dialer.Dial("tcp", r.URL.Hostname()+":"+port)
	if err != nil {
		log.Fatal(err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	defer socksConn.Close()
	httpConn, _, err := hijack.Hijack()
	if err != nil {
		log.Fatal(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer httpConn.Close()
	if r.Method == http.MethodConnect {
		httpConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	} else {
		r.Write(socksConn)
	}

	msgRead := make(chan string)
	msgWrite := make(chan string)
	pipeConn := func(w, r net.Conn, ch chan<- string) {
		io.Copy(w, r)
		ch <- "done!"
	}

	go pipeConn(socksConn, httpConn, msgRead)
	go pipeConn(httpConn, socksConn, msgWrite)

	<-msgRead
	<-msgWrite
}

func main() {
	flag.Parse()

	socksURL, err := url.Parse(*raddr)
	if err != nil {
		log.Fatalln("proxy url parse error:", err)
	}
	socks5Dialer, err := proxy.FromURL(socksURL, proxy.Direct)
	if err != nil {
		log.Fatalln("can not make proxy dialer:", err)
	}
	if err := http.ListenAndServe(*laddr, &HttpProxyRoutineHandler{Dialer: socks5Dialer}); err != nil {
		log.Fatalln("can not start http server:", err)
	}
}
