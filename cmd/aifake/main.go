// Command aifake runs the scripted fake OpenAI-compatible server
// (internal/ai/aifake) for local browser runs of the AI assistant. It is a
// test tool: there is no model behind it. Binds to 127.0.0.1 only.
package main

import (
	"flag"
	"log"
	"net/http"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/ai/aifake"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8199", "listen address (loopback only)")
	key := flag.String("key", "", "required bearer key (empty = none)")
	flag.Parse()
	if !strings.HasPrefix(*addr, "127.0.0.1:") && !strings.HasPrefix(*addr, "localhost:") {
		log.Fatal("aifake listens on loopback only")
	}
	log.Printf("aifake: scripted OpenAI-compatible server on http://%s/v1", *addr)
	log.Fatal(http.ListenAndServe(*addr, aifake.New(nil, *key)))
}
