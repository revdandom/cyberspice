package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// fileConfig is the on-disk shape of the config file (TOML).
// TOML is the de-facto config format in the Go CLI world: typed scalars,
// comments, and a clean 1:1 mapping to this flat struct.
type fileConfig struct {
	Style  string  `toml:"style"`  // led | solid | braille
	Color  string  `toml:"color"`  // classic | synthwave
	Amp    string  `toml:"amp"`    // linear | stevens | db
	Tilt   float64 `toml:"tilt"`   // spectral tilt, dB/octave
	Gain   float64 `toml:"gain"`   // initial gain multiplier
	Bands  int     `toml:"bands"`  // 0 = auto-size to terminal
	Chrome bool    `toml:"chrome"` // show header/footer bars on startup
	Fall   bool    `toml:"fall"`   // peak marker falls after its hold
}

// configPath is <user config dir>/cyberspec/config
// (~/.config/cyberspec/config on Linux).
func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cyberspec", "config"), nil
}

// loadConfigInto overlays any keys present in the config file onto o. A
// missing or unreadable file is not an error — the defaults just stand.
// Only keys actually written in the file take effect (md.IsDefined), so a
// value of 0 / false / "" is distinguishable from "absent".
func loadConfigInto(o *options) {
	path, err := configPath()
	if err != nil {
		return
	}
	var fc fileConfig
	md, err := toml.DecodeFile(path, &fc)
	if err != nil {
		return
	}
	if md.IsDefined("style") {
		o.barStyle = strings.ToLower(fc.Style)
	}
	if md.IsDefined("color") {
		o.scheme = schemeFromName(fc.Color)
	}
	if md.IsDefined("amp") {
		o.ampMode = normalizeAmp(fc.Amp)
	}
	if md.IsDefined("tilt") {
		o.tilt = fc.Tilt
	}
	if md.IsDefined("gain") {
		o.gain = fc.Gain
	}
	if md.IsDefined("bands") {
		o.bands = fc.Bands
	}
	if md.IsDefined("chrome") {
		o.chrome = fc.Chrome
	}
	if md.IsDefined("fall") {
		o.peakFall = fc.Fall
	}
}

// writeConfig writes o to the config file, creating the directory if needed.
// Returns the path written.
func writeConfig(o options) (string, error) {
	path, err := configPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	fc := fileConfig{
		Style:  o.barStyle,
		Color:  schemeName(o.scheme),
		Amp:    o.ampMode,
		Tilt:   o.tilt,
		Gain:   o.gain,
		Bands:  o.bands,
		Chrome: o.chrome,
		Fall:   o.peakFall,
	}

	var buf bytes.Buffer
	buf.WriteString("# cyberspec config — written with 'w'. Edit freely; re-read on next launch.\n")
	buf.WriteString("# CLI flags override these values.\n\n")
	if err := toml.NewEncoder(&buf).Encode(fc); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
