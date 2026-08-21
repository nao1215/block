// Command fakegh serves the offline fake GitHub (internal/fakegh) for the
// atago end-to-end suite.
//
// Usage: fakegh -addr 127.0.0.1:0 -url-file /path/to/url
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/nao1215/block/internal/fakegh"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "listen address")
	urlFile := flag.String("url-file", "", "file to write the server URL to once listening")
	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	s := fakegh.New(fakegh.Fixtures())
	s.SetBase(base)
	if *urlFile != "" {
		if err := os.WriteFile(*urlFile, []byte(base), 0o600); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("fakegh listening on %s", base)
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.Serve(ln))
}
