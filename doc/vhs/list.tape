# Discovery: which tools exist for a chain, answered offline from the registry
# snapshot embedded in the binary. Rendered to doc/img/list.gif.
#
# Nothing here reads block.toml, calls an API or installs anything — which is
# why a misspelled ecosystem can answer with the names that do exist.
#
# Regenerate with "make demo".

Output doc/img/list.gif

Set Shell "bash"
Set FontSize 18
Set Width 1360
Set Height 420
Set Padding 24
Set Theme "Dracula"
Set TypingSpeed 45ms

Hide
Type "export PS1='$ ' PATH=$PWD/dist:$PATH BLOCK_HOME=/tmp/block-demo"
Enter
Type "cd /tmp && clear"
Enter
Show
Sleep 800ms

# One blockchain system, and the commands each tool puts on your PATH.
Type "block list cosmos"
Sleep 500ms
Enter
Sleep 4s

# A typo answers with the systems that exist, rather than with nothing.
Type "block list etheruem"
Sleep 500ms
Enter
Sleep 3.5s
