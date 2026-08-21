// Command faketool stands in for a real blockchain CLI in the end-to-end
// suite. It is a compiled program rather than the `#!/bin/sh` script the fake
// GitHub server serves on Unix, because Windows cannot run one: an archive
// member named forge.exe has to be something the operating system will
// execute.
//
// It has no configuration. Which tool it is and which version it claims to be
// are read from where it was installed, because that is the one thing block
// guarantees about it:
//
//	$BLOCK_HOME/tools/<name>/<version>-<digest12>/[<subdir>/]<command>[.exe]
//
// so the command name is its own file name, and the version is the nearest
// ancestor directory named "<version>-<twelve hex digits>" — nearest, because
// a recipe may put its executables in a subdirectory of the install. A tool
// found anywhere else reports "unknown", which no scenario expects, so a
// layout change fails loudly instead of passing with a plausible string.
//
// Its behaviour matches the shell script byte for byte — same lines, same
// exit codes — so a scenario reads the same on either.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// serveTimeout bounds a --serve or --hang run so a scenario that forgets to
// signal the process cannot hang the suite.
const serveTimeout = 30 * time.Second

// interrupted is the exit status --serve reports when it handles a signal.
// The shell script uses 7, and the scenarios assert it.
const interrupted = 7

func main() {
	name, version := identity()
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "--exit":
			os.Exit(exitStatus(args))
		case "--serve":
			// A long-running process that shuts down cleanly, the way a node
			// or a local test network does.
			serve(name, true)
			return
		case "--hang":
			// The same with no handler at all: the signal itself ends it.
			serve(name, false)
			return
		}
	}
	fmt.Printf("%s %s (fake)\n", name, version)
	if len(args) > 0 {
		fmt.Printf("args: %s\n", strings.Join(args, " "))
	}
}

// identity reads the command name and the version out of the install path.
func identity() (name, version string) {
	self, err := os.Executable()
	if err != nil {
		return "unknown", "unknown"
	}
	name = strings.TrimSuffix(filepath.Base(self), ".exe")
	for dir := filepath.Dir(self); ; {
		if v, ok := versionOf(filepath.Base(dir)); ok {
			return name, v
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return name, "unknown"
		}
		dir = parent
	}
}

// installDir matches "<version>-<digest12>", the name block gives an install.
var installDir = regexp.MustCompile(`^(.+)-[0-9a-f]{12}$`)

// versionOf reads the version out of an install directory's name.
func versionOf(base string) (string, bool) {
	m := installDir.FindStringSubmatch(base)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// exitStatus reads the status "--exit N" asks for.
func exitStatus(args []string) int {
	if len(args) < 2 {
		return 0
	}
	code, err := strconv.Atoi(args[1])
	if err != nil {
		return 0
	}
	return code
}

// serve blocks until a signal arrives. With handle, it reports the shutdown
// and exits 7; without, it lets the signal end the process, so the parent
// sees 128+signal.
func serve(name string, handle bool) {
	if !handle {
		fmt.Printf("%s ready\n", name)
		select {
		case <-time.After(serveTimeout):
		}
		return
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	fmt.Printf("%s ready\n", name)
	select {
	case <-sig:
		fmt.Printf("%s stopping\n", name)
		os.Exit(interrupted)
	case <-time.After(serveTimeout):
	}
}
