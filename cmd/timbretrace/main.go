// Command timbretrace is a throwaway diagnostic: it prints per-segment spectral
// centroid and high-band energy so a timbre shift over the recording can be
// located in time.
package main

import (
	"context"
	"fmt"
	"math"
	"os"

	"github.com/666feiyu666/noise-detection/internal/audio"
)

const (
	frameSize = 2048
	hop       = 1024
	segSec    = 4.0
	highHz    = 6000.0
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: timbretrace <audio>")
		os.Exit(2)
	}
	ctx := context.Background()
	path := os.Args[1]
	meta, err := audio.Probe(ctx, path)
	if err != nil {
		panic(err)
	}
	pcm, err := audio.Decode(ctx, path, meta.SampleRate, meta.Channels)
	if err != nil {
		panic(err)
	}
	rate := float64(pcm.SampleRate)
	ch := pcm.Channels
	n := len(pcm.Samples) / ch
	mono := make([]float64, n)
	for i := 0; i < n; i++ {
		var s float64
		for c := 0; c < ch; c++ {
			s += float64(pcm.Samples[i*ch+c])
		}
		mono[i] = s / float64(ch)
	}
	win := hann(frameSize)

	type seg struct{ sumC, sumH, cnt float64 }
	segs := map[int]*seg{}
	for start := 0; start+frameSize <= n; start += hop {
		re := make([]float64, frameSize)
		im := make([]float64, frameSize)
		for i := 0; i < frameSize; i++ {
			re[i] = mono[start+i] * win[i]
		}
		fft(re, im)
		half := frameSize / 2
		var sumMag, sumW, tot, hi float64
		for k := 1; k < half; k++ {
			m := math.Hypot(re[k], im[k])
			f := float64(k) * rate / frameSize
			sumMag += m
			sumW += f * m
			tot += m * m
			if f >= highHz {
				hi += m * m
			}
		}
		t := float64(start+frameSize/2) / rate
		si := int(t / segSec)
		s := segs[si]
		if s == nil {
			s = &seg{}
			segs[si] = s
		}
		if sumMag > 0 {
			s.sumC += sumW / sumMag
		}
		if tot > 0 {
			s.sumH += hi / tot
		}
		s.cnt++
	}
	total := float64(n) / rate
	for si := 0; si <= int(total/segSec); si++ {
		s := segs[si]
		if s == nil || s.cnt == 0 {
			continue
		}
		hd := -300.0
		if m := s.sumH / s.cnt; m > 0 {
			hd = 10 * math.Log10(m)
		}
		fmt.Printf("t=%6.1f-%6.1f  centroid=%7.1f Hz  highband=%6.1f dB\n",
			float64(si)*segSec, math.Min(float64(si+1)*segSec, total), s.sumC/s.cnt, hd)
	}
}

func hann(n int) []float64 {
	w := make([]float64, n)
	for i := 0; i < n; i++ {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1))
	}
	return w
}

func fft(re, im []float64) {
	n := len(re)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		ang := -2 * math.Pi / float64(length)
		wr, wi := math.Cos(ang), math.Sin(ang)
		half := length / 2
		for i := 0; i < n; i += length {
			cr, ci := 1.0, 0.0
			for k := 0; k < half; k++ {
				a, b := i+k, i+k+half
				tr := cr*re[b] - ci*im[b]
				ti := cr*im[b] + ci*re[b]
				re[b] = re[a] - tr
				im[b] = im[a] - ti
				re[a] += tr
				im[a] += ti
				cr, ci = cr*wr-ci*wi, cr*wi+ci*wr
			}
		}
	}
}
