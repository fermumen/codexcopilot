package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/fermumen/codexcopilot/internal/auth"
	"github.com/fermumen/codexcopilot/internal/codex"
	"github.com/fermumen/codexcopilot/internal/copilot"
	"github.com/fermumen/codexcopilot/internal/paths"
	"github.com/fermumen/codexcopilot/internal/proxy"
)

const (
	defaultHost      = "127.0.0.1"
	defaultPort      = 11435
	defaultBaseURL   = "http://127.0.0.1:11435/v1/"
	serviceName      = "codexcopilot.service"
	vanillaFlagUsage = "strip Codex extras (Codex Apps connectors, plugins, bundled skills, web search, workspace dependencies)"
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: codexcopilot <auth|models|provider|responses-server|install-server-service|codex|launch> ...")
	os.Exit(2)
}

var runExternalCommand = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

var runForegroundCommand = func(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
	model := fs.String("model", "", "model id")
	host := fs.String("host", defaultHost, "listen host")
	port := fs.Int("port", defaultPort, "listen port")
	clientID := fs.String("client-id", "", "GitHub OAuth client id")
	enterpriseURL := fs.String("enterprise-url", "", "GitHub Enterprise URL or domain")
	vanilla := fs.Bool("vanilla", false, vanillaFlagUsage)
	_ = fs.Parse(args)
	p := paths.Default()
	a, err := ensureAuth(p, *clientID, *enterpriseURL)
	if err != nil {
		return err
	}
	remoteModels, err := copilot.FetchModels(a)
	if err != nil {
		return err
	}
	return runManagedResponsesServer(p, a, remoteModels, *model, *host, *port, *vanilla)
}

func systemdQuote(arg string) string {
	arg = strings.ReplaceAll(arg, `%`, `%%`)
	arg = strings.ReplaceAll(arg, `\`, `\\`)
	arg = strings.ReplaceAll(arg, `"`, `\"`)
	return `"` + arg + `"`
}

func serverServiceUnit(binaryPath, host string, port int, model string, codexHome string, vanilla bool) string {
	args := []string{
		systemdQuote(binaryPath),
		"responses-server",
		"--host",
		systemdQuote(host),
		"--port",
		fmt.Sprint(port),
	}
	if model != "" {
		args = append(args, "--model", systemdQuote(model))
	}
	if vanilla {
		args = append(args, "--vanilla")
	}
	lines := []string{
		"[Unit]",
		"Description=codexcopilot Responses proxy",
		"After=network-online.target",
		"Wants=network-online.target",
		"",
		"[Service]",
	}
	if codexHome != "" {
		lines = append(lines, "Environment="+systemdQuote("CODEX_HOME="+codexHome))
	}
	lines = append(lines,
		"ExecStart="+strings.Join(args, " "),
		"Restart=on-failure",
		"RestartSec=2",
		"",
		"[Install]",
		"WantedBy=default.target",
		"",
	)
	return strings.Join(lines, "\n")
}

func commandInstallServerService(args []string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("install-server-service is only supported on Linux systems with systemd")
	}
	fs := flag.NewFlagSet("install-server-service", flag.ExitOnError)
	model := fs.String("model", "", "model id")
	host := fs.String("host", defaultHost, "listen host")
	port := fs.Int("port", defaultPort, "listen port")
	binaryPath := fs.String("binary", "", "codexcopilot executable path")
	vanilla := fs.Bool("vanilla", false, vanillaFlagUsage)
	_ = fs.Parse(args)
	if *host == "" {
		return fmt.Errorf("--host cannot be empty")
	}
	if *port < 1 || *port > 65535 {
		return fmt.Errorf("--port must be between 1 and 65535")
	}
	p := paths.Default()
	currentAuth, err := auth.Load(p)
	if err != nil {
		return err
	}
	if currentAuth == nil {
		return fmt.Errorf("no saved GitHub Copilot login; run codexcopilot auth login before installing the service")
	}
	exe := *binaryPath
	if exe == "" {
		exe, err = os.Executable()
		if err != nil {
			return err
		}
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}
	serviceDir := filepath.Join(paths.ConfigHome(), "systemd", "user")
	servicePath := filepath.Join(serviceDir, serviceName)
	if err := os.MkdirAll(serviceDir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(servicePath, []byte(serverServiceUnit(exe, *host, *port, *model, os.Getenv("CODEX_HOME"), *vanilla)), 0o644); err != nil {
		return err
	}
	if out, err := runExternalCommand("systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl --user daemon-reload failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	if out, err := runExternalCommand("systemctl", "--user", "enable", "--now", serviceName); err != nil {
		return fmt.Errorf("systemctl --user enable --now %s failed: %w\n%s", serviceName, err, strings.TrimSpace(string(out)))
	}
	fmt.Printf("Installed and started %s.\n", serviceName)
	fmt.Printf("Unit: %s\n", servicePath)
	fmt.Printf("Proxy: http://%s:%d/v1/\n", *host, *port)
	return nil
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

func configureProviderFromModels(p paths.Paths, baseURL string, requestedModel string, remoteModels []copilot.Model, vanilla bool) (string, error) {
	models := copilot.CodexAppModels(remoteModels)
	if len(models) == 0 {
		return "", fmt.Errorf("no OpenAI Responses API models usable by Codex were returned from %s", baseURL)
	}
	selected, err := copilot.ChooseModel(models, requestedModel)
	if err != nil {
		return "", err
	}
	if err := codex.Configure(p, selected, models, baseURL, vanilla); err != nil {
		return "", err
	}
	return selected, nil
}

func splitPassthroughArgs(args []string) ([]string, []string) {
	for i, arg := range args {
		if arg == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

func startProxy(a auth.Auth, host string, port int) (*http.Server, <-chan error, string, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, "", err
	}
	server := &http.Server{Addr: addr, Handler: proxy.New(a)}
	errs := make(chan error, 1)
	go func() {
		errs <- server.Serve(listener)
	}()
	return server, errs, fmt.Sprintf("http://%s:%d", host, port), nil
}

func restoreProvider(p paths.Paths) {
	if _, err := codex.Restore(p); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to restore Codex provider settings: %v\n", err)
	}
}

var waitForServerShutdown = waitForServer

func waitForServer(server *http.Server, errs <-chan error) error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case <-signals:
		if err := server.Close(); err != nil {
			return err
		}
		err := <-errs
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case err := <-errs:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func runManagedResponsesServer(p paths.Paths, a auth.Auth, remoteModels []copilot.Model, requestedModel string, host string, port int, vanilla bool) error {
	server, errs, baseURL, err := startProxy(a, host, port)
	if err != nil {
		return err
	}
	selected, err := configureProviderFromModels(p, baseURL, requestedModel, remoteModels, vanilla)
	if err != nil {
		_ = server.Close()
		return err
	}
	defer restoreProvider(p)
	fmt.Printf("GitHub Copilot Responses proxy listening on %s/v1/\n", baseURL)
	fmt.Printf("Patched Codex default provider for %q.\n", selected)
	fmt.Println("Leave this process running while Codex uses GitHub Copilot. Config will be restored on exit.")
	return waitForServerShutdown(server, errs)
}

func commandCodex(args []string) error {
	toolArgs, codexArgs := splitPassthroughArgs(args)
	fs := flag.NewFlagSet("codex", flag.ExitOnError)
	model := fs.String("model", "", "model id")
	host := fs.String("host", defaultHost, "listen host")
	port := fs.Int("port", defaultPort, "listen port")
	clientID := fs.String("client-id", "", "GitHub OAuth client id")
	enterpriseURL := fs.String("enterprise-url", "", "GitHub Enterprise URL or domain")
	codexBin := fs.String("codex-bin", "codex", "codex executable path")
	vanilla := fs.Bool("vanilla", false, vanillaFlagUsage)
	_ = fs.Parse(toolArgs)
	codexArgs = append(fs.Args(), codexArgs...)
	p := paths.Default()
	a, err := ensureAuth(p, *clientID, *enterpriseURL)
	if err != nil {
		return err
	}
	remoteModels, err := copilot.FetchModels(a)
	if err != nil {
		return err
	}
	server, errs, baseURL, err := startProxy(a, *host, *port)
	if err != nil {
		return err
	}
	defer func() {
		_ = server.Close()
	}()
	selected, err := configureProviderFromModels(p, baseURL, *model, remoteModels, *vanilla)
	if err != nil {
		_ = server.Close()
		return err
	}
	defer restoreProvider(p)
	fmt.Printf("Configured Codex profile %q for %q.\n", codex.ProfileName, selected)
	fmt.Printf("GitHub Copilot proxy listening on %s/v1/\n", baseURL)
	runArgs := append([]string{"--profile", codex.ProfileName}, codexArgs...)
	runErr := runForegroundCommand(*codexBin, runArgs...)
	_ = server.Close()
	select {
	case err := <-errs:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	default:
	}
	return runErr
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
		vanilla := fs.Bool("vanilla", false, vanillaFlagUsage)
		_ = fs.Parse(args[1:])
		normalizedBase := codex.NormalizeProviderBaseURL(*baseURL)
		remoteModels, err := copilot.FetchModelsFromBaseURL(normalizedBase)
		if err != nil {
			return err
		}
		selected, err := configureProviderFromModels(p, normalizedBase, *model, remoteModels, *vanilla)
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
	vanilla := fs.Bool("vanilla", false, vanillaFlagUsage)
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
	server, errs, baseURL, err := startProxy(a, *host, *port)
	if err != nil {
		return err
	}
	selected, err := configureProviderFromModels(p, baseURL, *model, remoteModels, *vanilla)
	if err != nil {
		_ = server.Close()
		return err
	}
	defer restoreProvider(p)
	fmt.Printf("Configured Codex App profile %q at %s.\n", selected, p.CodexConfig)
	fmt.Printf("GitHub Copilot proxy listening on %s/v1/\n", baseURL)
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
	case "install-server-service":
		err = commandInstallServerService(os.Args[2:])
	case "codex":
		err = commandCodex(os.Args[2:])
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
