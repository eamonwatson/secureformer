package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// players get the next color from this list in join order
var playerColors = []uint8{9, 10, 12, 13, 14, 11, 1, 2, 4, 5, 6}

// I chose for quickchat to prevent people from doing things people will do with too much freedom
var quickChats = []string{"hi!", "bye", "oops", "gg"}

const maxNameLen = 12

type Player struct {
	ID    int
	Name  string
	Color uint8
	Score int

	x, y, vx, vy    float64
	onGround        bool
	coyote, jumpBuf int
	left, right     bool

	chat      string
	chatUntil uint64
	lastChat  uint64
}

type World struct {
	mu      sync.RWMutex
	level   *Level
	players map[int]*Player
	nextID  int
	coins   map[int]uint64
	log     []string
	tick    atomic.Uint64
	version atomic.Uint64
}

func NewWorld(l *Level) *World {
	return &World{level: l, players: map[int]*Player{}, coins: map[int]uint64{}}
}

func (w *World) Version() uint64 { return w.version.Load() }

func (w *World) Run(ctx context.Context) {
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.step()
		}
	}
}

func (w *World) Join(name string) (*Player, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.players) >= 64 {
		return nil, errors.New("server is full, try again later")
	}
	w.nextID++
	p := &Player{
		ID:    w.nextID,
		Name:  w.uniqueName(cleanName(name)),
		Color: playerColors[w.nextID%len(playerColors)],
	}
	w.respawn(p)
	w.players[p.ID] = p
	w.addLog(p.Name + " joined")
	return p, nil
}

func (w *World) Leave(id int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p, ok := w.players[id]; ok {
		delete(w.players, id)
		w.addLog(p.Name + " left")
	}
}

func (w *World) SetInput(id int, left, right bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p, ok := w.players[id]; ok {
		p.left, p.right = left, right
	}
}

func (w *World) Jump(id int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p, ok := w.players[id]; ok {
		p.jumpBuf = 3
	}
}

func (w *World) Say(id, n int) {
	if n < 0 || n >= len(quickChats) {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[id]
	now := w.tick.Load()
	if !ok || (p.lastChat != 0 && now-p.lastChat < 20) {
		return
	}
	p.chat, p.chatUntil, p.lastChat = quickChats[n], now+100, now
	w.addLog(p.Name + ": " + p.chat)
}

func (w *World) step() {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.tick.Add(1)
	if len(w.players) == 0 {
		return
	}
	l := w.level
	changed := false
	for idx, t := range w.coins {
		if t <= now {
			delete(w.coins, idx)
			changed = true
		}
	}
	for _, p := range w.players {
		ox, oy := p.x, p.y
		if p.chatUntil == now {
			changed = true
		}

		p.vx = 0
		if p.left {
			p.vx -= 12
		}
		if p.right {
			p.vx += 12
		}
		p.vy = math.Min(p.vy+50*0.05, 30)
		if p.jumpBuf > 0 {
			p.jumpBuf--
			if p.coyote > 0 {
				p.vy = -22
				p.coyote, p.jumpBuf = 0, 0
			}
		}

		dx, dy := p.vx*0.05, p.vy*0.05
		n := math.Ceil(math.Max(math.Abs(dx), math.Abs(dy)) / 0.5)
		p.onGround = false
		for range int(n) {
			w.moveX(p, dx/n)
			w.moveY(p, dy/n)
		}
		if p.onGround {
			p.coyote = 3
		} else if p.coyote > 0 {
			p.coyote--
		}

		cx, cy := int(p.x+0.5), int(p.y+0.5)
		switch t := l.at(cx, cy); {
		case p.y >= float64(l.H), t == '^':
			w.respawn(p)
		case t == 'o':
			if _, gone := w.coins[cy*l.W+cx]; !gone {
				w.coins[cy*l.W+cx] = now + 400
				p.Score++
				changed = true
			}
		}
		if p.x != ox || p.y != oy {
			changed = true
		}
	}
	if changed {
		w.version.Add(1)
	}
}

func (w *World) moveX(p *Player, dx float64) {
	if dx == 0 {
		return
	}
	p.x += dx
	tx := int(math.Floor(p.x))
	if dx > 0 {
		tx = int(math.Floor(p.x + 1 - 1e-6))
	}
	for ty := int(math.Floor(p.y)); ty <= int(math.Floor(p.y+1-1e-6)); ty++ {
		if w.level.at(tx, ty) != '#' {
			continue
		}
		if dx > 0 {
			p.x = float64(tx - 1)
		} else {
			p.x = float64(tx + 1)
		}
		p.vx = 0
		return
	}
}

func (w *World) moveY(p *Player, dy float64) {
	if dy == 0 {
		return
	}
	prevBottom := p.y + 1
	p.y += dy
	ty := int(math.Floor(p.y))
	if dy > 0 {
		ty = int(math.Floor(p.y + 1 - 1e-6))
	}
	for tx := int(math.Floor(p.x)); tx <= int(math.Floor(p.x+1-1e-6)); tx++ {
		t := w.level.at(tx, ty)
		if dy > 0 && (t == '#' || (t == '=' && prevBottom <= float64(ty)+1e-6)) {
			p.y, p.vy, p.onGround = float64(ty-1), 0, true
			return
		}
		if dy < 0 && t == '#' {
			p.y, p.vy = float64(ty+1), 0
			return
		}
	}
}

func (w *World) respawn(p *Player) {
	p.x, p.y = w.level.spawnX, w.level.spawnY
	p.vx, p.vy, p.coyote, p.jumpBuf = 0, 0, 0, 0
}

// used to generate a unique name per player preventing duplicates on the leaderboard
func (w *World) uniqueName(name string) string {
	taken := func(n string) bool {
		for _, p := range w.players {
			if p.Name == n {
				return true
			}
		}
		return false
	}
	n := name
	for i := 2; taken(n); i++ {
		suffix := fmt.Sprint(i)
		n = name[:min(len(name), maxNameLen-len(suffix))] + suffix
	}
	return n
}

func (w *World) addLog(s string) {
	w.log = append(w.log, s)
	if len(w.log) > 2 {
		w.log = w.log[len(w.log)-2:]
	}
	w.version.Add(1)
}

func cleanName(s string) string {
	var b strings.Builder
	for i := 0; i < len(s) && b.Len() < maxNameLen; i++ {
		if c := s[i]; c >= 0x20 && c < 0x7f {
			b.WriteByte(c)
		}
	}
	if out := strings.TrimSpace(b.String()); out != "" {
		return out
	}
	return "guest"
}

type cell struct {
	r  rune
	fg uint8
}

// draws one player's view, runs up to about 20 times a second per player
func (w *World) Render(id, width, height int, footer string) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	me, ok := w.players[id]
	if !ok || width < 20 || height < 8 {
		return "terminal too small"
	}
	l := w.level
	now := w.tick.Load()

	vw, vh := min(width, 160), min(height-4, 48)
	camX := camera(int(me.x+0.5)-vw/2, l.W, vw)
	camY := camera(int(me.y+0.5)-vh/2, l.H, vh)
	grid := make([]cell, vw*vh)
	put := func(x, y int, r rune, fg uint8) {
		if sx, sy := x-camX, y-camY; sx >= 0 && sx < vw && sy >= 0 && sy < vh {
			grid[sy*vw+sx] = cell{r, fg}
		}
	}

	for y := max(camY, 0); y < min(camY+vh, l.H); y++ {
		for x := max(camX, 0); x < min(camX+vw, l.W); x++ {
			switch l.tiles[y*l.W+x] {
			case '#':
				put(x, y, '█', 8)
			case '=':
				put(x, y, '=', 3)
			case '^':
				put(x, y, '^', 1)
			case 'o':
				if _, gone := w.coins[y*l.W+x]; !gone {
					put(x, y, 'o', 11)
				}
			}
		}
	}

	ps := make([]*Player, 0, len(w.players))
	for _, p := range w.players {
		ps = append(ps, p)
	}
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].Score != ps[j].Score {
			return ps[i].Score > ps[j].Score
		}
		return ps[i].ID < ps[j].ID
	})
	for _, p := range ps {
		px, py := int(p.x+0.5), int(p.y+0.5)
		if p.chatUntil > now {
			for i, r := range p.chat {
				put(px-len(p.chat)/2+i, py-1, r, 15)
			}
		}
		if p != me {
			put(px, py, rune(p.Name[0]), p.Color)
		}
	}
	put(int(me.x+0.5), int(me.y+0.5), '@', me.Color)

	var b strings.Builder
	b.Grow(vw*vh*2 + 512)

	var board strings.Builder
	board.WriteString("top:")
	for i, p := range ps[:min(3, len(ps))] {
		fmt.Fprintf(&board, " %d.%s %d", i+1, p.Name, p.Score)
	}
	left := width
	for _, seg := range []struct {
		s  string
		fg uint8
	}{
		{" SecureFormer  ", 7},
		{fmt.Sprintf("%d online", len(ps)), 10},
		{fmt.Sprintf("  score %d  ", me.Score), 7},
		{board.String(), 12},
	} {
		n := min(len(seg.s), left)
		writeFg(&b, seg.fg)
		b.WriteString(seg.s[:n])
		left -= n
	}
	b.WriteString("\x1b[0m\n")

	for sy := range vh {
		cur := uint8(255)
		for _, c := range grid[sy*vw : (sy+1)*vw] {
			if c.r == 0 {
				b.WriteByte(' ')
				continue
			}
			if c.fg != cur {
				writeFg(&b, c.fg)
				cur = c.fg
			}
			b.WriteRune(c.r)
		}
		b.WriteString("\x1b[0m\n")
	}

	for i := range 2 {
		s := ""
		if j := len(w.log) - 2 + i; j >= 0 {
			s = " " + w.log[j]
		}
		writeLine(&b, s, width, 8)
	}
	b.WriteString("\x1b[37m")
	b.WriteString(footer[:min(len(footer), width)])
	b.WriteString("\x1b[0m")
	return b.String()
}

func camera(c, size, view int) int {
	if size <= view {
		return -(view - size) / 2
	}
	return max(0, min(c, size-view))
}

func writeFg(b *strings.Builder, fg uint8) {
	if fg < 8 {
		fmt.Fprintf(b, "\x1b[%dm", 30+fg)
	} else {
		fmt.Fprintf(b, "\x1b[%dm", 82+fg)
	}
}

func writeLine(b *strings.Builder, s string, width int, fg uint8) {
	writeFg(b, fg)
	b.WriteString(s[:min(len(s), width)])
	b.WriteString("\x1b[0m\n")
}
