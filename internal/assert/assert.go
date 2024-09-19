package assert

import (
	"log"
	"runtime/debug"
)

func Assert(b bool) {
	Printf(b, "assertion failed\n")
}

func Printf(b bool, format string, a ...interface{}) {
	if !b {
		debug.PrintStack()
		log.Fatalf(format, a...)
	}
}
