// Command fusion-hub is an AI hub: a REPL for chatting with language models
// and running AIL apps, and an HTTP API that streams chat replies.
//
//	fusion-hub                    interactive REPL
//	fusion-hub <command ...>      run a command, then continue in the REPL
//	fusion-hub serve              HTTP API (POST /chat-with-ai)
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/api"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/chat"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/cli"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/commands"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/config"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/output"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/tts"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "fusion-hub:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	paths, err := config.DefaultPaths()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.Root, 0o755); err != nil {
		return err
	}
	settings, err := config.Load(paths.SettingsFile())
	if err != nil {
		return err
	}
	out := output.New(os.Stdout)
	reg := commands.NewRegistry()
	env := &commands.Env{
		Out:      out,
		Settings: settings,
		Paths:    paths,
		Chat:     chat.NewManager(paths, settings),
		Speaker:  tts.New(settings, paths.Dir(config.TTS)),
		State:    &commands.State{},
		Registry: reg,
	}

	if len(args) > 0 && (args[0] == "serve" || args[0] == "--serve") {
		return serve(env)
	}
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Println("usage: fusion-hub [command ...] | fusion-hub serve\nType \"help\" inside the REPL for commands.")
		return nil
	}
	repl := &cli.REPL{Env: env, Registry: reg}
	return repl.Run(context.Background(), strings.Join(args, " "))
}

func serve(env *commands.Env) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := api.New(api.ConfigFrom(env.Settings), func(ctx context.Context, sessionID, message string, w io.Writer) error {
		_, err := env.Chat.Send(ctx, sessionID, message, w, chat.SendOptions{})
		return err
	})
	env.Out.Log("Listening for /chat-with-ai requests...")
	return srv.ListenAndServe(ctx, func(name string, port int) {
		env.Out.Log("%s is listening on port %d", name, port)
	})
}
