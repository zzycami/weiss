package weiss

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"github.com/elazarl/goproxy"
	"io"
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
	log.Printf("[weiss] Start port=%s jsonBytes=%d", port, len(jsonData))
	configCache := make(map[string]*tls.Config, 0)
	// Handshake must finish quickly; tunnel reads need long idle (login/reCAPTCHA can go quiet >8s).
	handshakeDeadline := 45 * time.Second
	tunnelReadDeadline := 10 * time.Minute
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
		if err != nil {
			log.Printf("[weiss] JSON parse warn: %v (continuing with empty hardMap)", err)
		} else {
			m := f.(map[string]interface{})
			for k, v := range m {
				hardMap[k] = v.(string)
			}
			log.Printf("[weiss] hardMap entries=%d", len(hardMap))
		}
	} else {
		log.Printf("[weiss] no JSON hardMap (ECH/OneZero only)")
	}
	for k, v := range hardMap {
		OneZeroCache.Data[k] = v
		log.Printf("[weiss] seed cache %s -> %s", k, v)
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
				log.Printf("[weiss] CONNECT panic host=%s recover=%v", ctx.Req.URL.Host, recover)
				_, _ = conn.Write([]byte("HTTP/1.1 500"))
			}
			conn.Close()
		}()
		host := ctx.Req.URL.Hostname()
		log.Printf("[weiss] CONNECT begin host=%s full=%s", host, ctx.Req.URL.Host)
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
		_ = tlsCon.SetDeadline(time.Now().Add(handshakeDeadline))
		log.Printf("[weiss] MITM TLS handshake (client side) host=%s", host)
		if err := tlsCon.Handshake(); err != nil {
			log.Printf("[weiss] MITM TLS handshake failed host=%s err=%v", host, err)
			panic(err)
		}
		_ = tlsCon.SetDeadline(time.Time{})
		defer tlsCon.Close()
		clientWriter := bufio.NewReadWriter(bufio.NewReader(tlsCon), bufio.NewWriter(tlsCon))

		remoteCon, echConfigList := buildOneZeroCon(ctx, hardMap)
		if remoteCon == nil {
			log.Printf("[weiss] buildOneZeroCon returned nil host=%s", host)
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
		_ = remote.SetDeadline(time.Now().Add(handshakeDeadline))
		log.Printf("[weiss] upstream TLS handshake host=%s ech=%t", host, len(echConfigList) > 0)
		if err := remote.Handshake(); err != nil {
			log.Printf("[weiss] upstream TLS handshake failed host=%s err=%v", host, err)
			panic(err)
		}
		_ = remote.SetDeadline(time.Time{})
		defer remote.Close()
		remoteWriter := bufio.NewReadWriter(bufio.NewReader(remote), bufio.NewWriter(remote))
		log.Printf("[weiss] tunnel ready host=%s (read idle deadline=%v per direction; was 8s and broke quiet pages)", host, tunnelReadDeadline)
		channel := make(chan error, 2)
		go func() {
			buffer := make([]byte, 32768)
			var err error
			var n int
			for {
				_ = tlsCon.SetDeadline(time.Now().Add(tunnelReadDeadline))
				n, err = clientWriter.Read(buffer)
				if err != nil {
					log.Printf("[weiss] tunnel client->upstream STOP host=%s readErr=%v bytes=%d", host, err, n)
					break
				}
				if n == 0 {
					continue
				}
				_, err = remoteWriter.Write(buffer[:n])
				if err != nil {
					log.Printf("[weiss] tunnel client->upstream STOP host=%s writeErr=%v", host, err)
					break
				}
				if err := remoteWriter.Flush(); err != nil {
					log.Printf("[weiss] tunnel client->upstream STOP host=%s flushErr=%v", host, err)
					break
				}
			}
			channel <- err
		}()
		go func() {
			buffer := make([]byte, 32768)
			var err error
			var n int
			for {
				_ = remote.SetDeadline(time.Now().Add(tunnelReadDeadline))
				n, err = remoteWriter.Read(buffer)
				if err != nil {
					log.Printf("[weiss] tunnel upstream->client STOP host=%s readErr=%v bytes=%d", host, err, n)
					break
				}
				if n == 0 {
					continue
				}
				_, err = clientWriter.Write(buffer[:n])
				if err != nil {
					log.Printf("[weiss] tunnel upstream->client STOP host=%s writeErr=%v", host, err)
					break
				}
				if err := clientWriter.Flush(); err != nil {
					log.Printf("[weiss] tunnel upstream->client STOP host=%s flushErr=%v", host, err)
					break
				}
			}
			channel <- err
		}()
		e1 := <-channel
		e2 := <-channel
		if e1 != nil {
			log.Printf("[weiss] tunnel first leg done host=%s err=%v", host, e1)
		}
		if e2 != nil {
			log.Printf("[weiss] tunnel second leg done host=%s err=%v", host, e2)
		}
		if e1 != nil && !errors.Is(e1, io.EOF) {
			panic(e1)
		}
		if e2 != nil && !errors.Is(e2, io.EOF) {
			panic(e2)
		}
		log.Printf("[weiss] CONNECT end host=%s (tunnel closed)", host)
	})
	if len(blackPorts) > 0 {
		proxy.OnRequest(
			goproxy.ReqHostIs(blackPorts...),
		).HandleConnect(goproxy.AlwaysReject)
	}
	proxy.Verbose = true
	server = &http.Server{Addr: ":" + port, Handler: proxy}
	go func() {
		log.Printf("[weiss] ListenAndServe :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[weiss] ListenAndServe stopped: %v", err)
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
		remoteCon, err := net.Dial("tcp", dialAddr)
		if err != nil {
			log.Printf("[weiss] buildOneZeroCon ECH dial failed host=%s addr=%s err=%v", host, dialAddr, err)
		} else {
			log.Printf("[weiss] buildOneZeroCon path=ECH host=%s dial=%s echBytes=%d", host, dialAddr, len(echList))
			return remoteCon, echList
		}
	} else {
		log.Printf("[weiss] buildOneZeroCon ECH unavailable host=%s (fallback)", host)
	}

	OneZeroCache.Lock.RLock()
	data, ok := OneZeroCache.Data[host]
	OneZeroCache.Lock.RUnlock()
	if ok {
		remoteCon, err := net.Dial("tcp", data+portSuffix)
		if err != nil {
			log.Printf("[weiss] buildOneZeroCon OneZero cache dial fail host=%s addr=%s err=%v", host, data+portSuffix, err)
			return nil, nil
		}
		log.Printf("[weiss] buildOneZeroCon path=OneZeroCache host=%s addr=%s", host, data+portSuffix)
		return remoteCon, nil
	}

	if v, ok := hardMap[host]; ok {
		remoteCon, err := net.Dial("tcp", v+portSuffix)
		OneZeroCache.Lock.Lock()
		OneZeroCache.Data[host] = v
		OneZeroCache.Lock.Unlock()
		if err != nil {
			log.Printf("[weiss] buildOneZeroCon hardMap dial fail host=%s addr=%s err=%v", host, v+portSuffix, err)
			return nil, nil
		}
		log.Printf("[weiss] buildOneZeroCon path=hardMap host=%s addr=%s", host, v+portSuffix)
		return remoteCon, nil
	}

	log.Printf("[weiss] buildOneZeroCon path=OneZero DNS fetch host=%s", host)
	oneZeroReq := OneZeroReq{host}
	res, err := oneZeroReq.fetch()
	if err != nil {
		log.Printf("[weiss] OneZero fetch panic host=%s err=%v", host, err)
		panic(err)
	}
	for _, answer := range res.Answer {
		if answer.Type != 1 {
			continue
		}
		remoteCon, err := net.Dial("tcp", answer.Data+portSuffix)
		if err != nil {
			log.Printf("[weiss] buildOneZeroCon OneZero DNS dial fail host=%s addr=%s err=%v", host, answer.Data+portSuffix, err)
			return nil, nil
		}
		OneZeroCache.Lock.Lock()
		OneZeroCache.Data[host] = answer.Data
		OneZeroCache.Lock.Unlock()
		log.Printf("[weiss] buildOneZeroCon path=OneZeroDNS host=%s addr=%s", host, answer.Data+portSuffix)
		return remoteCon, nil
	}
	log.Printf("[weiss] buildOneZeroCon no route host=%s", host)
	return nil, nil
}
