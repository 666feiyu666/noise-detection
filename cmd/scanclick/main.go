// Command scanclick is a throwaway diagnostic: it lists sample-to-sample
// discontinuities (click/pop candidates) inside an optional time range, so a
// suspected burst can be compared with a false positive elsewhere.
package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"

	"github.com/666feiyu666/noise-detection/internal/audio"
)

type pk struct{ t, d float64 }

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: scanclick <audio> [startSec] [endSec] [minDiff]")
		os.Exit(2)
	}
	ctx := context.Background()
	path := os.Args[1]
	start, end, minDiff := 0.0, math.MaxFloat64, 0.2
	if len(os.Args) >= 4 {
		start, _ = strconv.ParseFloat(os.Args[2], 64)
		end, _ = strconv.ParseFloat(os.Args[3], 64)
	}
	if len(os.Args) >= 5 {
		minDiff, _ = strconv.ParseFloat(os.Args[4], 64)
	}

	meta, err := audio.Probe(ctx, path)
	if err != nil {
		panic(err)
	}
	pcm, err := audio.Decode(ctx, path, meta.SampleRate, meta.Channels)
	if err != nil {
		panic(err)
	}
	rate, ch := pcm.SampleRate, pcm.Channels
	n := len(pcm.Samples) / ch

	// second-order jump: a click is a spike in the derivative, but a repeating
	// tone also produces large first differences; report both so they can be told
	// apart.
	var peaks []pk
	for f := 2; f < n; f++ {
		t := float64(f) / float64(rate)
		if t < start || t > end {
			continue
		}
		var dmax, ddmax float64
		for c := 0; c < ch; c++ {
			prev := float64(pcm.Samples[(f-1)*ch+c])
			cur := float64(pcm.Samples[f*ch+c])
			prev2 := float64(pcm.Samples[(f-2)*ch+c])
			if d := math.Abs(cur - prev); d > dmax {
				dmax = d
			}
			if dd := math.Abs(cur - 2*prev + prev2); dd > ddmax {
				ddmax = dd
			}
		}
		if dmax >= minDiff {
			peaks = append(peaks, pk{t: t, d: dmax})
		}
		_ = ddmax
	}
	sort.Slice(peaks, func(i, j int) bool { return peaks[i].t < peaks[j].t })
	fmt.Printf("window [%.2f, %.2f]s, frames with diff>=%.2f: %d\n", start, end, minDiff, len(peaks))
	limit := 60
	for i, p := range peaks {
		if i >= limit {
			fmt.Printf("  ... (%d more)\n", len(peaks)-limit)
			break
		}
		fmt.Printf("  t=%9.4fs diff=%.3f\n", p.t, p.d)
	}
	best, bestT := 0, 0.0
	for i := 0; i < len(peaks); i++ {
		c := 0
		for j := i; j < len(peaks) && peaks[j].t-peaks[i].t <= 0.04; j++ {
			c++
		}
		if c > best {
			best, bestT = c, peaks[i].t
		}
	}
	fmt.Printf("densest 40 ms window: %d frames starting t=%.4fs\n", best, bestT)
}
