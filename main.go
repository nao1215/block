// Command block locks a project's blockchain toolchain: declare tools in
// block.toml, pin them in block.lock, and reproduce them anywhere.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/nao1215/block/cmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cmd.Execute(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
