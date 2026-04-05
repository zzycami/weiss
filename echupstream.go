package weiss

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Same HK endpoint as iOS PixivHelper AppApiEchTransport.echConfigBaseURL.
const defaultEchConfigAPI = "http://47.239.145.80:8092/ech/config"

type echAPIResponse struct {
	Host      string   `json:"host"`
	EchConfig *string  `json:"echConfig"`
	Ipv4hint  []string `json:"ipv4hint"`
	TTL       int64    `json:"ttl"`
	Source    string   `json:"source"`
	Error     *string  `json:"error"`
}

type echCached struct {
	list      []byte
	ips       []string
	expiresAt time.Time
}

var (
	echMu          sync.Mutex
	echByHost      = map[string]*echCached{} // lowercase hostname
	echHTTPClient  = &http.Client{Timeout: 12 * time.Second}
	echConfigAPI   = defaultEchConfigAPI // mutable for tests
)

func isLikelyEchConfigList(b []byte) bool {
	if len(b) < 2 {
		return false
	}
	declared := int(b[0])<<8 | int(b[1])
	return declared == len(b)-2
}

func echCacheTTL(apiTTL int64) time.Duration {
	if apiTTL > 0 && apiTTL < 86400 {
		return time.Duration(apiTTL) * time.Second
	}
	return 15 * time.Minute
}

func getEchCached(host string) (*echCached, bool) {
	echMu.Lock()
	defer echMu.Unlock()
	e, ok := echByHost[host]
	if !ok || time.Now().After(e.expiresAt) {
		if ok {
			delete(echByHost, host)
		}
		return nil, false
	}
	return e, true
}

func setEchCached(host string, list []byte, ips []string, ttl time.Duration) {
	echMu.Lock()
	defer echMu.Unlock()
	echByHost[host] = &echCached{
		list:      list,
		ips:       ips,
		expiresAt: time.Now().Add(ttl),
	}
}

func fetchEchFromAPI(hostname string) ([]byte, []string, int64, error) {
	u, err := url.Parse(echConfigAPI)
	if err != nil {
		return nil, nil, 0, err
	}
	q := u.Query()
	q.Set("host", hostname)
	u.RawQuery = q.Encode()

	log.Printf("[weiss-ech] GET %s", u.String())
	resp, err := echHTTPClient.Get(u.String())
	if err != nil {
		log.Printf("[weiss-ech] GET err host=%s %v", hostname, err)
		return nil, nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[weiss-ech] HTTP status=%d host=%s", resp.StatusCode, hostname)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		log.Printf("[weiss-ech] read body err host=%s %v", hostname, err)
		return nil, nil, 0, err
	}
	var parsed echAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		log.Printf("[weiss-ech] JSON unmarshal err host=%s bodyPrefix=%q err=%v", hostname, trimForLog(body, 120), err)
		return nil, nil, 0, err
	}
	if parsed.EchConfig == nil || *parsed.EchConfig == "" {
		log.Printf("[weiss-ech] empty echConfig host=%s source=%s", hostname, parsed.Source)
		return nil, nil, parsed.TTL, fmt.Errorf("ech: empty echConfig")
	}
	raw, err := base64.StdEncoding.DecodeString(*parsed.EchConfig)
	if err != nil || len(raw) == 0 {
		return nil, nil, parsed.TTL, err
	}
	if !isLikelyEchConfigList(raw) {
		return nil, nil, parsed.TTL, errors.New("ech: invalid ECHConfigList")
	}
	var hints []string
	for _, s := range parsed.Ipv4hint {
		t := strings.TrimSpace(s)
		if t != "" {
			hints = append(hints, t)
		}
	}
	log.Printf("[weiss-ech] ok host=%s hints=%v ttl=%d rawLen=%d", hostname, hints, parsed.TTL, len(raw))
	return raw, hints, parsed.TTL, nil
}

func trimForLog(b []byte, max int) string {
	s := string(b)
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

// tryECHUpstream returns TLS ECH config list bytes and a TCP dial address (host:port).
// If the HK API has no ECH for this hostname, ok is false and callers should use OneZero / plain DNS.
func tryECHUpstream(hostname, portSuffix string) (echList []byte, dialAddr string, ok bool) {
	h := strings.ToLower(strings.TrimSpace(hostname))
	if h == "" {
		return nil, "", false
	}
	if e, hit := getEchCached(h); hit {
		addr := resolveDialAddr(h, e.ips, portSuffix)
		if addr == "" {
			log.Printf("[weiss-ech] cache hit but resolveDialAddr empty host=%s hints=%v", h, e.ips)
			return nil, "", false
		}
		log.Printf("[weiss-ech] cache HIT host=%s dial=%s echLen=%d", h, addr, len(e.list))
		return e.list, addr, true
	}

	list, hints, apiTTL, err := fetchEchFromAPI(h)
	if err != nil || len(list) == 0 {
		if err != nil {
			log.Printf("[weiss-ech] fetch miss host=%s err=%v", h, err)
		} else {
			log.Printf("[weiss-ech] fetch miss host=%s empty list", h)
		}
		return nil, "", false
	}

	addr := resolveDialAddr(h, hints, portSuffix)
	if addr == "" {
		log.Printf("[weiss-ech] resolveDialAddr empty after fetch host=%s hints=%v", h, hints)
		return nil, "", false
	}

	setEchCached(h, list, hints, echCacheTTL(apiTTL))
	return list, addr, true
}

func resolveDialAddr(hostname string, ipv4hints []string, portSuffix string) string {
	if len(ipv4hints) > 0 {
		return ipv4hints[0] + portSuffix
	}
	addrs, err := net.LookupHost(hostname)
	if err != nil || len(addrs) == 0 {
		return ""
	}
	return addrs[0] + portSuffix
}
