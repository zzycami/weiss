// Command weissd runs the local MITM CONNECT proxy for manual testing (curl, browsers, etc.).
//
// Example:
//
//	go run ./cmd/weissd 8091
//	curl -vk --http1.1 -x http://127.0.0.1:8091 'https://app-api.pixiv.net/'
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
