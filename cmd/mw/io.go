package main

import "os"

// stderr is the destination for non-content diagnostic output (refresh
// summaries, banners, etc.). Kept separate from stdout so the read path's
// output is clean for piping if Patrick ever wants that.
var stderr = os.Stderr
