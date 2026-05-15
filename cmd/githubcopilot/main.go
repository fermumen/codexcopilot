package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/local/ghc-launch-codex/internal/auth"
	"github.com/local/ghc-launch-codex/internal/codex"
	"github.com/local/ghc-launch-codex/internal/copilot"
	"github.com/local/ghc-launch-codex/internal/paths"
	"github.com/local/ghc-launch-codex/internal/proxy"
)

const (
	defaultHost = "127.0.0.1"
	defaultPort = 11435
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: githubcopilot <auth|models|serve|launch> ...")
	os.Exit(2)
}

func ensureAuth(p paths.Paths, clientID, enterpriseURL string) (auth.Auth, error) {
	current, err := auth.Load(p)
	if err != nil {
		return auth.Auth{}, err
	}
	if current != nil && (enterpriseURL == "" || current.EnterpriseURL == strings.TrimRight(strings.TrimPrefix(strings.TrimPrefix(enterpriseURL, "https://"), "http://"), "/")) {
		return *current, nil
	}
	next, err := auth.Login(p, clientID, enterpriseURL)
	if err != nil {
		return auth.Auth{}, err
	}
	return *next, nil
}

func commandAuth(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("auth requires login, logout, or status")
	}
	p := paths.Default()
	switch args[0] {
	case "login":
		fs := flag.NewFlagSet("auth login", flag.ExitOnError)
		clientID := fs.String("client-id", "", "GitHub OAuth client id")
		enterpriseURL := fs.String("enterprise-url", "", "GitHub Enterprise URL or domain")
		_ = fs.Parse(args[1:])
		if _, err := auth.Login(p, *clientID, *enterpriseURL); err != nil {
			return err
		}
		fmt.Println("GitHub Copilot login saved.")
	case "logout":
		removed, err := auth.Logout(p)
		if err != nil {
			return err
		}
		if removed {
			fmt.Println("Removed saved GitHub Copilot login.")
		} else {
			fmt.Println("No saved login found.")
		}
	case "status":
		current, err := auth.Load(p)
		if err != nil {
			return err
		}
		if current == nil {
			fmt.Println("No saved GitHub Copilot login.")
			os.Exit(1)
		}
		scope := current.EnterpriseURL
		if scope == "" {
			scope = "github.com"
		}
		fmt.Printf("Saved GitHub Copilot login found for %s.\n", scope)
	default:
		return fmt.Errorf("unknown auth command %q", args[0])
	}
	return nil
}

func commandModels(args []string) error {
	fs := flag.NewFlagSet("models", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "print raw JSON")
	clientID := fs.String("client-id", "", "GitHub OAuth client id")
	enterpriseURL := fs.String("enterprise-url", "", "GitHub Enterprise URL or domain")
	_ = fs.Parse(args)
	a, err := ensureAuth(paths.Default(), *clientID, *enterpriseURL)
	if err != nil {
		return err
	}
	models, err := copilot.FetchModels(a)
	if err != nil {
		return err
	}
	if *jsonOut {
		data, _ := json.MarshalIndent(models, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	for _, model := range models {
		id, _ := model["id"].(string)
		name, _ := model["name"].(string)
		if name == "" {
			name = id
		}
		fmt.Printf("%s\t%s\n", id, name)
	}
	return nil
}

func commandServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	host := fs.String("host", defaultHost, "listen host")
	port := fs.Int("port", defaultPort, "listen port")
	clientID := fs.String("client-id", "", "GitHub OAuth client id")
	enterpriseURL := fs.String("enterprise-url", "", "GitHub Enterprise URL or domain")
	_ = fs.Parse(args)
	a, err := ensureAuth(paths.Default(), *clientID, *enterpriseURL)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("%s:%d", *host, *port)
	fmt.Printf("GitHub Copilot proxy listening on http://%s/v1/\n", addr)
	return proxy.New(a).ListenAndServe(addr)
}

func commandLaunch(args []string) error {
	fs := flag.NewFlagSet("launch", flag.ExitOnError)
	model := fs.String("model", "", "model id")
	host := fs.String("host", defaultHost, "listen host")
	port := fs.Int("port", defaultPort, "listen port")
	configOnly := fs.Bool("config-only", false, "write Codex App config without starting the proxy")
	noLaunch := fs.Bool("no-launch", false, "do not launch Codex App")
	restore := fs.Bool("restore", false, "restore previous Codex App config")
	clientID := fs.String("client-id", "", "GitHub OAuth client id")
	enterpriseURL := fs.String("enterprise-url", "", "GitHub Enterprise URL or domain")
	_ = fs.Parse(args)
	target := strings.ToLower(strings.Join(fs.Args(), "-"))
	if target != "codex-app" {
		return fmt.Errorf("unknown launch target %q, supported target: codex-app", target)
	}
	p := paths.Default()
	if *restore {
		restored, err := codex.Restore(p)
		if err != nil {
			return err
		}
		if restored {
			fmt.Println("Restored Codex App config.")
		} else {
			fmt.Println("No Codex App config restore state found.")
		}
		if !*noLaunch && !*configOnly {
			if err := codex.LaunchApp(); err != nil {
				return err
			}
			fmt.Println("Requested Codex App launch.")
		}
		return nil
	}
	a, err := ensureAuth(p, *clientID, *enterpriseURL)
	if err != nil {
		return err
	}
	remoteModels, err := copilot.FetchModels(a)
	if err != nil {
		return err
	}
	models := copilot.CodexAppModels(remoteModels)
	if len(models) == 0 {
		return fmt.Errorf("GitHub Copilot returned no OpenAI Responses API models usable by Codex App")
	}
	selected, err := copilot.ChooseModel(models, *model)
	if err != nil {
		return err
	}
	baseURL := fmt.Sprintf("http://%s:%d", *host, *port)
	if err := codex.Configure(p, selected, models, baseURL); err != nil {
		return err
	}
	fmt.Printf("Configured Codex App profile %q at %s.\n", selected, p.CodexConfig)
	if *configOnly {
		return nil
	}
	server := &http.Server{Addr: fmt.Sprintf("%s:%d", *host, *port), Handler: proxy.New(a)}
	errs := make(chan error, 1)
	go func() {
		fmt.Printf("GitHub Copilot proxy listening on %s/v1/\n", baseURL)
		errs <- server.ListenAndServe()
	}()
	if !*noLaunch {
		if err := codex.LaunchApp(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		} else {
			fmt.Println("Requested Codex App launch.")
		}
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signals:
		return server.Close()
	case err := <-errs:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "auth":
		err = commandAuth(os.Args[2:])
	case "models":
		err = commandModels(os.Args[2:])
	case "serve":
		err = commandServe(os.Args[2:])
	case "launch":
		err = commandLaunch(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		log.Fatal(err)
	}
}
