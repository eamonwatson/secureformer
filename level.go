package main

import (
	_ "embed"
	"strings"
)

//go:embed level.txt
var levelData string

type Level struct {
	W, H           int
	tiles          []byte
	spawnX, spawnY float64
}

func loadLevel(src string) *Level {
	lines := strings.Split(strings.TrimRight(src, "\n"), "\n")
	l := &Level{H: len(lines)}
	for _, line := range lines {
		l.W = max(l.W, len(line))
	}
	l.tiles = make([]byte, l.W*l.H)
	for y, line := range lines {
		line += strings.Repeat(" ", l.W-len(line))
		if x := strings.IndexByte(line, '@'); x >= 0 {
			l.spawnX, l.spawnY = float64(x), float64(y)
			line = strings.Replace(line, "@", " ", 1)
		}
		copy(l.tiles[y*l.W:], line)
	}
	return l
}

func (l *Level) at(x, y int) byte {
	switch {
	case x < 0 || x >= l.W || y < 0:
		return '#'
	case y >= l.H:
		return ' '
	}
	return l.tiles[y*l.W+x]
}
