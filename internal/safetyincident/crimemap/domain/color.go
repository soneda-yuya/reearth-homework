package domain

// ColorFromCount maps a per-country incident count to a fixed 6-stop sequential
// palette (colorbrewer "Reds"). `max` is the highest count across the map so
// shades are normalised relative to the busiest country — a uniform-low map
// stays light, a single-spike map still highlights the spike.
//
// max == 0 or count == 0 always returns the empty-grey so a quiet map does not
// show red.
func ColorFromCount(count, max int) string {
	if count == 0 || max == 0 {
		return "#f0f0f0"
	}
	// Quintile buckets against max. Using integer arithmetic keeps the split
	// deterministic and avoids a floats-in-switch rounding wobble that a
	// property test previously caught.
	switch {
	case count*5 <= max*1:
		return "#fee5d9"
	case count*5 <= max*2:
		return "#fcae91"
	case count*5 <= max*3:
		return "#fb6a4a"
	case count*5 <= max*4:
		return "#de2d26"
	default:
		return "#a50f15"
	}
}
