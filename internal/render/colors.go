package render

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/desertthunder/documango/internal/theme"
)

// Semantic colors in the order of their conventional base16 slots: red is
// base08, orange base09, and so on up to purple at base0E.
var (
	semanticNames = [7]string{"red", "orange", "yellow", "green", "cyan", "blue", "purple"}
	semanticHues  = [7]float64{0, 30, 50, 120, 180, 220, 275}
)

const (
	// minChroma is the spread between the strongest and weakest RGB channel,
	// out of 255, below which a color reads as grey and is never matched.
	minChroma = 30
	// missCost is the hue distance charged when a semantic color keeps its
	// conventional slot because no accent is left to match it.
	missCost = 90
	// slotBonus favors the conventional slot when hues are close, keeping the
	// scheme author's choice for schemes that follow base16 conventions.
	slotBonus = 10
)

// schemeVars returns the palette variables of s followed by the semantic
// color variables that point at its slots.
func schemeVars(s theme.Scheme) string {
	var b strings.Builder
	b.WriteString(s.CSSVars())
	for i, slot := range semanticSlots(s.Palette) {
		fmt.Fprintf(&b, " --%s: var(--base%02X);", semanticNames[i], slot)
	}
	return b.String()
}

// semanticSlots picks, for each semantic color, the accent slot among
// base08..base0F whose hue is nearest, using each slot at most once and
// minimizing the total hue distance. Grey or unreadable colors are skipped.
func semanticSlots(p [16]string) [7]int {
	type candidate struct {
		slot int
		hue  float64
	}
	var cands []candidate
	for slot := 8; slot < 16; slot++ {
		if h, ok := hue(p[slot]); ok {
			cands = append(cands, candidate{slot, h})
		}
	}

	var cur, best [7]int
	for i := range best {
		best[i] = 8 + i
	}
	bestCost := math.Inf(1)
	used := make([]bool, len(cands))
	var walk func(t int, cost float64)
	walk = func(t int, cost float64) {
		if cost >= bestCost {
			return
		}
		if t == len(cur) {
			best, bestCost = cur, cost
			return
		}
		for i, c := range cands {
			if used[i] {
				continue
			}
			d := math.Abs(c.hue - semanticHues[t])
			d = min(d, 360-d)
			if c.slot == 8+t {
				d = max(0, d-slotBonus)
			}
			used[i], cur[t] = true, c.slot
			walk(t+1, cost+d)
			used[i] = false
		}
		cur[t] = 8 + t
		walk(t+1, cost+missCost)
	}
	walk(0, 0)
	return best
}

// hue returns the hue in degrees of a "#rrggbb" color, or false when the
// color cannot be parsed or is too close to grey to have a meaningful hue.
func hue(color string) (float64, bool) {
	rgb, err := strconv.ParseUint(strings.TrimPrefix(color, "#"), 16, 32)
	if err != nil || len(color) != 7 {
		return 0, false
	}
	r, g, b := float64(rgb>>16&0xff), float64(rgb>>8&0xff), float64(rgb&0xff)
	hi, lo := max(r, g, b), min(r, g, b)
	c := hi - lo
	if c < minChroma {
		return 0, false
	}
	var h float64
	switch hi {
	case r:
		h = math.Mod((g-b)/c+6, 6)
	case g:
		h = (b-r)/c + 2
	default:
		h = (r-g)/c + 4
	}
	return h * 60, true
}
