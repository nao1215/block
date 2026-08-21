package diag

// BLK5xxx: running a command with the locked toolchain, through `block exec`
// or through a shim. Neither of them resolves, downloads, installs or writes,
// so everything here is about what is already on disk and on PATH.
var (
	// CommandNotFound is a command neither the toolchain nor PATH provides.
	CommandNotFound = register(5001,
		"the command is in neither the toolchain nor PATH",
		"block exec runs any command with the locked tools first on PATH, not only the locked tools themselves. This one is not a command the project locks, and PATH has no other executable by that name either.",
		"Check the spelling, add the tool to block.toml and run \"block lock\" and \"block sync\", or install the command the usual way for your system.",
		"v0.1.0")

	// CommandFailedToStart is an executable that could not be launched.
	CommandFailedToStart = register(5002,
		"the command could not be started",
		"The executable was found but the operating system refused to run it: it is not executable, it is built for another architecture, or its interpreter is missing. Its own exit status, when it has one, is reported instead of this — a command that runs and fails is not a block error.",
		"Run \"block sync\" to reinstall the tool. If it persists, the artifact is not one this machine can run; check the platform the pin covers.",
		"v0.1.0")

	// ShimLoop is two shim directories handing a command back and forth.
	ShimLoop = register(5003,
		"shims are calling each other in a loop",
		"A shim runs the next command of its name on PATH when the current directory is not a block project. With more than one block shim directory on PATH, each one can find the other and the command never reaches a real tool.",
		"Remove the extra shim directory from PATH. There is one shim directory per store: $BLOCK_HOME/shims.",
		"v0.1.0")

	// ShimNoFallback is a shim outside a project with nothing to step aside to.
	ShimNoFallback = register(5004,
		"a shim found neither a project nor another command of that name",
		"The command was run through a block shim, but the working directory is not inside a block project — or the project does not lock a tool providing this command — and PATH holds no other executable of that name to step aside to.",
		"Change into a project that locks the tool, or install the command the usual way for your system so the shim has something to defer to.",
		"v0.1.0")
)
