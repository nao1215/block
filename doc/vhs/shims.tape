# The shims: two repositories, two Foundry versions, no switching. Rendered to
# doc/img/shims.gif.
#
# `block sync` writes one file per command into $BLOCK_HOME/shims; with that
# directory on PATH, `forge` is the version the project you are standing in
# locked. Nothing is stored in the shim, so there is one `forge` for every
# project and nothing to regenerate when you switch branches.
#
# Regenerate with "make demo".

Output doc/img/shims.gif

Set Shell "bash"
Set FontSize 18
Set Width 1180
Set Height 400
Set Padding 24
Set Theme "Dracula"
Set TypingSpeed 45ms

Hide
Type "export PS1='$ ' BLOCK_HOME=/tmp/block-demo PATH=/tmp/block-demo/shims:$PWD/dist:$PATH"
Enter
Type "cd doc/demo && clear"
Enter
Show
Sleep 800ms

# Two projects, each pinning its own Foundry.
Type "grep foundry defi/block.toml bridge/block.toml"
Sleep 500ms
Enter
Sleep 2.5s

# No "block exec", no activation, no shell hook: the directory decides.
Type "cd defi && forge --version"
Sleep 500ms
Enter
Sleep 3s

Type "cd ../bridge && forge --version"
Sleep 500ms
Enter
Sleep 3.5s
