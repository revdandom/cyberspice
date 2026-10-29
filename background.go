package main

import (
	"os"
	"strings"

	"github.com/muesli/termenv"
)

// resolveBackground turns the -bg / `background` setting into the colour
// fades end at (viz.SetBackground). "auto" asks the terminal for its
// background with an OSC 11 query; a terminal that doesn't answer (or a
// multiplexer termenv won't query) leaves the old black target. Anything
// else must be a "#rrggbb" colour. Runs before Bubble Tea takes the terminal,
// since the reply arrives on stdin.
func resolveBackground(setting string) string {
	if !strings.EqualFold(setting, "auto") && setting != "" {
		return setting
	}
	if c, ok := termenv.NewOutput(os.Stdout).BackgroundColor().(termenv.RGBColor); ok {
		return strings.ToLower(string(c))
	}
	return ""
}
