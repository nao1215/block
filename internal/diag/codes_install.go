package diag

// BLK4xxx: unpacking a verified artifact into the store and keeping the store
// honest about what is in it.
var (
	// ArchiveUnreadable is an archive block cannot unpack.
	ArchiveUnreadable = register(4001,
		"an artifact could not be unpacked",
		"The downloaded file is not an archive of a kind block extracts, or it is one but is damaged. block extracts .tar.gz, .tgz, .tar.bz2, .tbz2 and .zip; anything else is installed as a single raw executable.",
		"For a `[tools.<name>.source]` of your own, check that the asset template names the file the upstream actually publishes. For a registry tool, report it at https://github.com/nao1215/block-registry/issues.",
		"v0.1.0")

	// ExecutableMissing is an archive without one of the declared executables.
	ExecutableMissing = register(4002,
		"the artifact does not contain a declared executable",
		"The archive unpacked, but one of the executables the recipe promises is not in it — often because the upstream wraps everything in a versioned directory that strip_components has to drop, or because a binary was renamed.",
		"For a `[tools.<name>.source]` of your own, correct bin or strip_components against the archive's real layout. For a registry tool, report it at https://github.com/nao1215/block-registry/issues.",
		"v0.1.0")

	// InstallDamaged is an install that exists but cannot be trusted.
	InstallDamaged = register(4003,
		"an install in the store is incomplete or damaged",
		"The install directory exists but does not verify: its completion marker is missing, or one of its executables is gone or is not executable. block writes the marker last and renames the directory into place atomically, so this is an interrupted install, a half-restored CI cache, or something that deleted files under $BLOCK_HOME.",
		"Run \"block sync\" to replace it. block never runs an install it cannot verify.",
		"v0.1.0")

	// NotInstalled is a locked tool that was never installed.
	NotInstalled = register(4004,
		"a locked tool is not installed",
		"block.lock pins the tool and the pin is current, but nothing for it is in the store — a fresh clone, a cleared cache, or a runner without the store restored. exec and the shims never install: what they run is what sync put there.",
		"Run \"block sync\".",
		"v0.1.0")

	// StoreUnwritable is a store block cannot write to.
	StoreUnwritable = register(4005,
		"the store could not be written",
		"block could not create or replace a directory under $BLOCK_HOME. The store is a per-user directory; a shared or root-owned one, or a full disk, stops sync before anything is installed.",
		"Check the permissions and free space on $BLOCK_HOME (~/.local/share/block by default, %LOCALAPPDATA%\\block on Windows), or point BLOCK_HOME at a directory this user owns.",
		"v0.1.0")
)
