package diag

// BLK1xxx: what was asked for. The tools declared in block.toml, the pins
// recorded in block.lock, the disagreements between the two, and the names
// typed on the command line. Nothing here needs a network, and nothing here is
// block's own doing: the fix is always an edit or a different command.
var (
	// ManifestMissing is "block.toml not found".
	ManifestMissing = register(1001,
		"block.toml not found",
		"block works on a project, and a project is the directory tree under a block.toml. block looked for that file in the working directory and every directory above it, and found none.",
		"Write a block.toml in the repository root, or change into a directory inside a project that has one. The manifest is a few lines; see https://nao1215.github.io/block/getting-started/.",
		"v0.1.0")

	// ManifestInvalid is a block.toml that cannot be read as one.
	ManifestInvalid = register(1002,
		"block.toml is not valid",
		"block.toml was found but does not parse, or says something block cannot act on: an unknown key, a tool name that is not a tool name, a platform block does not support, or a version constraint that is not a dotted prefix. block refuses the whole file rather than acting on the part of it that made sense.",
		"Fix the key the message names. Constraints are dotted prefixes only — \"1\", \"1.7\", \"1.7.4\" — with no operators or ranges; the format is at https://nao1215.github.io/block/reference/.",
		"v0.1.0")

	// LockMissing is "block.lock not found".
	LockMissing = register(1003,
		"block.lock not found",
		"The project declares a toolchain but has never resolved it, so there is nothing to install or run. block will not resolve one on the spot: which version a build uses is a decision that gets committed, not one made by whoever happened to run the command.",
		"Run \"block lock\" to resolve block.toml into block.lock, and commit both files.",
		"v0.1.0")

	// LockInvalid is a block.lock that cannot be read as one.
	LockInvalid = register(1004,
		"block.lock is not valid",
		"block.lock does not parse, carries a format version this block does not write, or holds a value that is not one lock could have produced — a digest that is not a digest, a version with a path separator in it, a pin that does not satisfy the constraint it records. A lockfile arrives through pull requests and hand edits, so it is checked rather than trusted.",
		"Do not hand-edit block.lock. Restore it from version control, or delete it and run \"block lock\" to write it again. A lockfile from a newer block needs a newer block.",
		"v0.1.0")

	// LockStale is a lockfile that disagrees with the manifest.
	LockStale = register(1005,
		"block.lock does not match block.toml",
		"The manifest and the lockfile describe different toolchains: a tool was added, removed, or re-pointed at a different constraint or source since the lock was written. sync and exec both refuse rather than installing something nobody resolved, and every disagreement is listed, not just the first.",
		"Run \"block lock\" and commit the result. In CI, \"block lock --check\" reports the same disagreements before anything is installed.",
		"v0.1.0")

	// LockPlatformMissing is a lockfile with no artifact for this machine.
	LockPlatformMissing = register(1006,
		"block.lock has no artifact for this platform",
		"The lockfile pins the tool, but not for the operating system and architecture this machine is. That happens when the pin was made on a machine of a different kind and block.toml does not declare the platforms the project supports.",
		"Add every platform the project builds on to the \"platforms\" list in block.toml and run \"block lock\" again, so one lockfile covers the whole team and CI.",
		"v0.1.0")

	// UnknownTool is a manifest naming a tool that is nowhere.
	UnknownTool = register(1007,
		"the tool is not in the registry",
		"block.toml names a tool with no `[tools.<name>.source]` of its own, and the registry compiled into this binary has no recipe for that name.",
		"Run \"block list\" to see the names the registry carries, or define `[tools.<name>.source]` in block.toml to fetch the tool without waiting for a registry entry.",
		"v0.1.0")

	// ToolNotDeclared is a tool named on the command line that block.toml
	// does not declare.
	ToolNotDeclared = register(1008,
		"the named tool is not declared in block.toml",
		"`block lock <tool>` re-resolves the tools you name and keeps every other pin. A name that is not in block.toml would silently do nothing, so it is refused instead.",
		"Check the spelling against block.toml, or run \"block lock\" with no arguments to re-resolve every tool.",
		"v0.1.0")

	// UnknownEcosystem is a `block list` argument that names no blockchain
	// system the registry carries.
	UnknownEcosystem = register(1010,
		"the ecosystem is not one the registry knows",
		"`block list <ecosystem>` narrows the catalogue to one blockchain system. The name given is not one any recipe declares, so there is nothing to narrow to; the message lists the names that exist.",
		"Use one of the names in the message, or run `block list` with no argument to see every tool with the systems it serves.",
		"v0.1.0")

	// CommandConflict is one command name provided by two executables.
	CommandConflict = register(1009,
		"two executables would provide one command",
		"Two tools in the toolchain — or two paths inside one tool — end in the same command name. Which of them runs would depend on how it was called: a shim resolves a command through the lockfile, PATH resolves it by directory order, and the two can disagree. Names are compared without regard to case on every platform, because Windows resolves PATH that way and a lockfile is committed and read everywhere.",
		"Remove one of the two tools from block.toml, or pin the fork rather than the tool it forks.",
		"v0.1.0")
)
