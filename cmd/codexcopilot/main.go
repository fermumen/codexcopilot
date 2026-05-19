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

	"github.com/fermumen/codexcopilot/internal/auth"
	"github.com/fermumen/codexcopilot/internal/codex"
	"github.com/fermumen/codexcopilot/internal/copilot"
	"github.com/fermumen/codexcopilot/internal/paths"
	"github.com/fermumen/codexcopilot/internal/proxy"
)

const (
	defaultHost    = "127.0.0.1"
	defaultPort    = 11435
	defaultBaseURL = "http://127.0.0.1:11435/v1/"
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: codexcopilot <auth|models|provider|responses-server|launch> ...")
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
	fs := flag.NewFlagSet("responses-server", flag.ExitOnError)
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
	fmt.Printf("GitHub Copilot Responses proxy listening on http://%s/v1/\n", addr)
	fmt.Println("This mode does not write Codex App config or launch Codex App.")
	return proxy.New(a).ListenAndServe(addr)
}

func rejectOldLaunchFlags(args []string) error {
	rejected := map[string]bool{
		"config-only": true,
		"no-launch":   true,
		"server-only": true,
		"restore":     true,
	}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if eq := strings.Index(name, "="); eq >= 0 {
			name = name[:eq]
		}
		if rejected[name] {
			return fmt.Errorf("launch no longer supports --%s; use responses-server or provider commands instead", name)
		}
	}
	return nil
}

func splitLaunchArgs(args []string) ([]string, []string) {
	valueFlags := map[string]bool{
		"model":          true,
		"host":           true,
		"port":           true,
		"client-id":      true,
		"enterprise-url": true,
	}
	var flagArgs []string
	var targetArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			targetArgs = append(targetArgs, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if eq := strings.Index(name, "="); eq >= 0 {
			name = name[:eq]
		}
		flagArgs = append(flagArgs, arg)
		if valueFlags[name] && !strings.Contains(arg, "=") && i+1 < len(args) {
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}
	return flagArgs, targetArgs
}

func configureProviderFromModels(p paths.Paths, baseURL string, requestedModel string, remoteModels []copilot.Model) (string, error) {
	models := copilot.CodexAppModels(remoteModels)
	if len(models) == 0 {
		return "", fmt.Errorf("no OpenAI Responses API models usable by Codex were returned from %s", baseURL)
	}
	selected, err := copilot.ChooseModel(models, requestedModel)
	if err != nil {
		return "", err
	}
	if err := codex.Configure(p, selected, models, baseURL); err != nil {
		return "", err
	}
	return selected, nil
}

func commandProvider(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("provider requires patch or restore")
	}
	p := paths.Default()
	switch args[0] {
	case "patch":
		fs := flag.NewFlagSet("provider patch", flag.ExitOnError)
		baseURL := fs.String("base-url", defaultBaseURL, "OpenAI-compatible proxy base URL")
		model := fs.String("model", "", "model id")
		_ = fs.Parse(args[1:])
		normalizedBase := codex.NormalizeProviderBaseURL(*baseURL)
		remoteModels, err := copilot.FetchModelsFromBaseURL(normalizedBase)
		if err != nil {
			return err
		}
		selected, err := configureProviderFromModels(p, normalizedBase, *model, remoteModels)
		if err != nil {
			return err
		}
		fmt.Printf("Patched Codex provider settings for %q at %s.\n", selected, p.CodexConfig)
		fmt.Printf("Provider base URL: %s\n", normalizedBase)
	case "restore":
		restored, err := codex.Restore(p)
		if err != nil {
			return err
		}
		if restored {
			fmt.Println("Restored Codex provider settings.")
		} else {
			fmt.Println("No Codex provider restore state found.")
		}
	default:
		return fmt.Errorf("unknown provider command %q", args[0])
	}
	return nil
}

func commandLaunch(args []string) error {
	if err := rejectOldLaunchFlags(args); err != nil {
		return err
	}
	fs := flag.NewFlagSet("launch", flag.ExitOnError)
	model := fs.String("model", "", "model id")
	host := fs.String("host", defaultHost, "listen host")
	port := fs.Int("port", defaultPort, "listen port")
	clientID := fs.String("client-id", "", "GitHub OAuth client id")
	enterpriseURL := fs.String("enterprise-url", "", "GitHub Enterprise URL or domain")
	flagArgs, targetArgs := splitLaunchArgs(args)
	_ = fs.Parse(flagArgs)
	target := strings.ToLower(strings.Join(targetArgs, "-"))
	if target != "codex-app" {
		return fmt.Errorf("unknown launch target %q, supported target: codex-app", target)
	}
	p := paths.Default()
	a, err := ensureAuth(p, *clientID, *enterpriseURL)
	if err != nil {
		return err
	}
	remoteModels, err := copilot.FetchModels(a)
	if err != nil {
		return err
	}
	baseURL := fmt.Sprintf("http://%s:%d", *host, *port)
	selected, err := configureProviderFromModels(p, baseURL, *model, remoteModels)
	if err != nil {
		return err
	}
	fmt.Printf("Configured Codex App profile %q at %s.\n", selected, p.CodexConfig)
	server := &http.Server{Addr: fmt.Sprintf("%s:%d", *host, *port), Handler: proxy.New(a)}
	errs := make(chan error, 1)
	go func() {
		fmt.Printf("GitHub Copilot proxy listening on %s/v1/\n", baseURL)
		errs <- server.ListenAndServe()
	}()
	if err := codex.LaunchApp(); err != nil {
		_ = server.Close()
		return err
	}
	fmt.Println("Requested Codex App launch.")
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
	case "responses-server":
		err = commandServe(os.Args[2:])
	case "provider":
		err = commandProvider(os.Args[2:])
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
