package diag

// BLK9xxx: block's own faults. A reader who sees one of these has found a bug;
// there is nothing in their project to fix.
var (
	// Internal is a failure block cannot attribute to anything the user did.
	Internal = register(9001,
		"an internal error",
		"block reached a state it does not have an explanation for. This is a bug in block, not something wrong with the project, the network or the upstream.",
		"Report it at https://github.com/nao1215/block/issues/new with the command you ran and the whole message.",
		"v0.1.0")
)
