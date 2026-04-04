package weiss

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/elazarl/goproxy"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

var (
	OneZeroCache = struct {
		Data map[string]string
		Lock sync.RWMutex
	}{make(map[string]string), sync.RWMutex{}}
	server *http.Server
)

// connectHostMatches accepts CONNECT targets with or without an explicit :443 suffix.
func connectHostMatches(allowed []string) goproxy.ReqConditionFunc {
	set := make(map[string]struct{}, len(allowed)*2)
	for _, name := range allowed {
		set[name] = struct{}{}
		set[name+":443"] = struct{}{}
	}
	return func(req *http.Request, _ *goproxy.ProxyCtx) bool {
		_, ok := set[req.URL.Host]
		return ok
	}
}

func Start(port string, jsonData string) {
	configCache := make(map[string]*tls.Config, 0)
	DELAY := 8 * time.Second
	whiteList := []string{
		"pixiv.net",
		"www.pixiv.net",
		"app-api.pixiv.net",
		"oauth.secure.pixiv.net",
		"source.pixiv.net",
		"accounts.pixiv.net",
		"touch.pixiv.net",
		"imgaz.pixiv.net",
		"dic.pixiv.net",
		"comic.pixiv.net",
		"factory.pixiv.net",
		"g-client-proxy.pixiv.net",
		"sketch.pixiv.net",
		"payment.pixiv.net",
		"sensei.pixiv.net",
		"novel.pixiv.net",
		"en-dic.pixiv.net",
		"i1.pixiv.net",
		"i2.pixiv.net",
		"i3.pixiv.net",
		"i4.pixiv.net",
		"d.pixiv.org",
		"fanbox.pixiv.net",
		"pixivsketch.net",
		"pximg.net",
		"i.pximg.net",
		"s.pximg.net",
		"pixiv.pximg.net",
	}
	hardMap := make(map[string]string)
	if len(jsonData) != 0 { //不支持map所以只能传json
		var f interface{}
		err := json.Unmarshal([]byte(jsonData), &f)
		if err == nil {
			m := f.(map[string]interface{})
			for k, v := range m {
				hardMap[k] = v.(string)
			}
		}
	}
	for k, v := range hardMap {
		OneZeroCache.Data[k] = v
		fmt.Println(k + v)
	}

	blackList := []string{}
	blackPorts := make([]string, len(blackList))
	for i, s := range blackList {
		blackPorts[i] = s + ":443"
	}

	proxy := goproxy.NewProxyHttpServer()
	proxy.OnRequest(
		connectHostMatches(whiteList),
	).HijackConnect(func(req *http.Request, conn net.Conn, ctx *goproxy.ProxyCtx) {
		defer func() {
			if recover := recover(); recover != nil {
				_, _ = conn.Write([]byte("HTTP/1.1 500"))
			}
			conn.Close()
		}()
		log.Println(ctx.Req.URL.Hostname())
		clientTLSConfig, err := func(host string) (*tls.Config, error) {
			if config, ok := configCache[host]; ok {
				return config, nil
			}
			config, err := goproxy.TLSConfigFromCA(&goproxy.GoproxyCa)(host, ctx)
			if err != nil {
				return nil, err
			}
			configCache[host] = config
			return config, nil
		}(ctx.Req.URL.Host)
		if err != nil {
			panic(err)
		}
		tlsCon := tls.Server(conn, clientTLSConfig)
		_ = tlsCon.SetDeadline(time.Now().Add(DELAY))
		if err := tlsCon.Handshake(); err != nil {
			panic(err)
		}
		defer tlsCon.Close()
		clientWriter := bufio.NewReadWriter(bufio.NewReader(tlsCon), bufio.NewWriter(tlsCon))

		remoteCon, echConfigList := buildOneZeroCon(ctx, hardMap)
		if remoteCon == nil {
			panic("Error host:" + ctx.Req.URL.Hostname())
		}
		defer remoteCon.Close()
		upstreamTLS := &tls.Config{
			ServerName: ctx.Req.URL.Hostname(),
			InsecureSkipVerify: true,
			VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
				return nil
			},
		}
		if len(echConfigList) > 0 {
			upstreamTLS.MinVersion = tls.VersionTLS13
			upstreamTLS.EncryptedClientHelloConfigList = echConfigList
		}
		remote := tls.Client(remoteCon, upstreamTLS)
		if err := remote.Handshake(); err != nil {
			panic(err)
		}
		defer remote.Close()
		remoteWriter := bufio.NewReadWriter(bufio.NewReader(remote), bufio.NewWriter(remote))
		channel := make(chan error)
		go func() {
			buffer := make([]byte, 1024)
			var err error
			for {
				_ = tlsCon.SetDeadline(time.Now().Add(DELAY))
				num, err := clientWriter.Read(buffer)
				if err != nil {
					break
				}
				_, err = remoteWriter.Write(buffer[:num])
				if err != nil {
					break
				}
				if err := remoteWriter.Flush(); err != nil {
					break
				}
			}
			channel <- err
		}()
		go func() {
			buffer := make([]byte, 1024)
			var err error
			for {
				_ = tlsCon.SetDeadline(time.Now().Add(DELAY))
				num, err := remoteWriter.Read(buffer)
				if err != nil {
					break
				}
				_, err = clientWriter.Write(buffer[:num])
				if err != nil {
					break
				}
				if err := clientWriter.Flush(); err != nil {
					break
				}
			}
			channel <- err
		}()
		if err := <-channel; err != nil {
			panic(err)
		}
		if err := <-channel; err != nil {
			panic(err)
		}
	})
	if len(blackPorts) > 0 {
		proxy.OnRequest(
			goproxy.ReqHostIs(blackPorts...),
		).HandleConnect(goproxy.AlwaysReject)
	}
	proxy.Verbose = true
	server = &http.Server{Addr: ":" + port, Handler: proxy}
	go func() {
		if err := server.ListenAndServe(); err != nil {
		}
	}()
}

func Close() {
	if server != nil {
		_ = server.Close()
	}
}

// buildOneZeroCon dials the upstream TCP endpoint. When the second return is non-empty,
// the TLS client must use crypto/tls ECH (TLS 1.3) with that EncryptedClientHelloConfigList.
// It prefers HK ech/config + ECH (same as iOS); if unavailable, it falls back to OneZero DNS IP dialing.
func buildOneZeroCon(ctx *goproxy.ProxyCtx, hardMap map[string]string) (net.Conn, []byte) {
	host := ctx.Req.URL.Hostname()
	portSuffix := ":443"
	if _, p, err := net.SplitHostPort(ctx.Req.Host); err == nil {
		portSuffix = ":" + p
	}

	if echList, dialAddr, ok := tryECHUpstream(host, portSuffix); ok {
		if remoteCon, err := net.Dial("tcp", dialAddr); err == nil {
			log.Println("weiss upstream ECH", host, "tcp", dialAddr)
			return remoteCon, echList
		}
	}

	OneZeroCache.Lock.RLock()
	data, ok := OneZeroCache.Data[host]
	OneZeroCache.Lock.RUnlock()
	if ok {
		remoteCon, err := net.Dial("tcp", data+portSuffix)
		if err != nil {
			return nil, nil
		}
		log.Println("weiss upstream OneZero cache", host)
		return remoteCon, nil
	}

	if v, ok := hardMap[host]; ok {
		remoteCon, err := net.Dial("tcp", v+portSuffix)
		OneZeroCache.Lock.Lock()
		OneZeroCache.Data[host] = v
		OneZeroCache.Lock.Unlock()
		if err != nil {
			return nil, nil
		}
		log.Println("weiss upstream hardMap", host)
		return remoteCon, nil
	}

	oneZeroReq := OneZeroReq{host}
	res, err := oneZeroReq.fetch()
	if err != nil {
		panic(err)
	}
	for _, answer := range res.Answer {
		if answer.Type != 1 {
			continue
		}
		remoteCon, err := net.Dial("tcp", answer.Data+portSuffix)
		if err != nil {
		}
		OneZeroCache.Lock.Lock()
		OneZeroCache.Data[host] = answer.Data
		OneZeroCache.Lock.Unlock()
		log.Println("weiss upstream OneZero DNS", host)
		return remoteCon, nil
	}
	return nil, nil
}
