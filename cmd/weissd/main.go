// Command weissd runs the local MITM CONNECT proxy for manual testing (curl, browsers, iOS Simulator, etc.).
//
// Simulator shares the Mac loopback: if weissd listens on 127.0.0.1:PORT, the app uses the same PORT
// via HttpProxySessionManager (and must skip in-app WeissStart — see pixiv-client WebViewController, env PIXIV_USE_EXTERNAL_WEISS=1).
//
// Example:
//
//	go run ./cmd/weissd 28492
//	curl -vk --http1.1 -x http://127.0.0.1:28492 'https://app-api.pixiv.net/'
//
// Optional third argument: same JSON as iOS `createWeissJsonValue()`: host→IP map plus optional `upstream_mode`:
//   "auto" (default) — try HK ech/config ECH, then domain fronting on ECH TLS failure;
//   "ech_only" — ECH only (no plain fallback);
//   "fronting_only" — skip ECH, use JSON / OneZero / DNS only.
//
//	go run ./cmd/weissd 28492 '{"app-api.pixiv.net":"210.140.139.155","upstream_mode":"fronting_only"}'
package main

import (
	"bufio"
	"fmt"
	"os"

	"weiss"
)

func main() {
	port := "8091"
	if len(os.Args) >= 2 {
		port = os.Args[1]
	}
	// Optional JSON map host -> IP (same as iOS WeissStart second argument).
	jsonData := ""
	if len(os.Args) >= 3 {
		jsonData = os.Args[2]
	}

	weiss.Start(port, jsonData)
	_, _ = fmt.Fprintf(os.Stderr, "weiss listening on :%s (HTTP proxy). MITM uses goproxy built-in CA.\n", port)
	_, _ = fmt.Fprintf(os.Stderr, "Try: curl -vk --http1.1 -x http://127.0.0.1:%s 'https://app-api.pixiv.net/'\n", port)
	_, _ = fmt.Fprintf(os.Stderr, "Press Enter to stop.\n")
	_, _ = bufio.NewReader(os.Stdin).ReadBytes('\n')
	weiss.Close()
}
