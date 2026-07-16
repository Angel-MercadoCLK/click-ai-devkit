package ui

// bannerArt is the CLICK-AI ANSI Shadow banner shown by `click install` (and any command that
// wants the full branded header). Kept verbatim — do not reflow or re-indent.
const bannerArt = ` ██████╗██╗     ██╗ ██████╗██╗  ██╗    █████╗ ██╗
██╔════╝██║     ██║██╔════╝██║ ██╔╝   ██╔══██╗██║
██║     ██║     ██║██║     █████╔╝    ███████║██║
██║     ██║     ██║██║     ██╔═██╗    ██╔══██║██║
╚██████╗███████╗██║╚██████╗██║  ██╗   ██║  ██║██║
 ╚═════╝╚══════╝╚═╝ ╚═════╝╚═╝  ╚═╝   ╚═╝  ╚═╝╚═╝`

// tagline is the dim subtitle printed under the banner.
const tagline = "AI Devkit · Click Seguros · v0.1"

// BannerArt returns the raw, uncolored CLICK-AI wordmark art so other packages can apply their
// own coloring (e.g. the interactive menu renders it in a white→orange ramp instead of this
// file's cyan→blue install-banner gradient). Kept as the single source of the wordmark glyphs.
func BannerArt() string { return bannerArt }

// bannerGradient is a cyan→blue ramp applied one color per banner line (a per-line gradient,
// not true per-character — tech-spec doesn't require finer granularity and this keeps rendering
// simple and deterministic).
var bannerGradient = []string{
	"#54F2F2",
	"#3FD6F0",
	"#2BB8ED",
	"#2B93E0",
	"#2E6FD1",
	"#3350C2",
}
