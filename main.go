package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/log/v2"
	"charm.land/ssh"
	"charm.land/wish/v2"
	"charm.land/wish/v2/activeterm"
	bm "charm.land/wish/v2/bubbletea"
	"charm.land/wish/v2/logging"
	"charm.land/wish/v2/ratelimiter"
	"golang.org/x/time/rate"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	addr := net.JoinHostPort(env("HOST", "0.0.0.0"), env("PORT", "23234"))
	world := NewWorld(loadLevel(levelData))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go world.Run(ctx)

	s, err := wish.NewServer(
		wish.WithAddress(addr),
		wish.WithHostKeyPath(env("HOST_KEY_PATH", ".ssh/id_ed25519")),
		wish.WithIdleTimeout(15*time.Minute),
		wish.WithMiddleware(
			bm.Middleware(handler(world)),
			activeterm.Middleware(),
			ratelimiter.Middleware(ratelimiter.NewRateLimiter(rate.Every(2*time.Second), 5, 4096)),
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Fatal("could not create server", "error", err)
	}

	log.Info("starting server", "addr", addr)
	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Error("server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("stopping server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(shutdownCtx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Error("shutdown error", "error", err)
	}
}

func handler(world *World) bm.Handler {
	return func(sess ssh.Session) (tea.Model, []tea.ProgramOption) {
		p, err := world.Join(sess.User())
		if err != nil {
			wish.Println(sess, err.Error())
			return nil, nil
		}
		go func() {
			<-sess.Context().Done()
			world.Leave(p.ID)
		}()
		pty, _, _ := sess.Pty()
		return &splash{text: "eamonw.nl", game: newModel(world, p.ID, pty.Window.Width, pty.Window.Height)}, []tea.ProgramOption{tea.WithFPS(20)}
	}
}
