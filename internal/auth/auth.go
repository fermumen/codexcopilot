package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fermumen/codexcopilot/internal/paths"
)

const DefaultClientID = "Ov23li8tweQw6odWQebz"

type Auth struct {
	AccessToken   string `json:"access_token"`
	EnterpriseURL string `json:"enterprise_url,omitempty"`
	TokenType     string `json:"token_type,omitempty"`
}

func clientID(value string) string {
	if value != "" {
		return value
	}
	if v := os.Getenv("GHC_COPILOT_CLIENT_ID"); v != "" {
		return v
	}
	return DefaultClientID
}

func normalizeDomain(value string) string {
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	return strings.TrimRight(value, "/")
}

func oauthURLs(enterpriseURL string) (string, string) {
	domain := "github.com"
	if enterpriseURL != "" {
		domain = normalizeDomain(enterpriseURL)
	}
	return "https://" + domain + "/login/device/code", "https://" + domain + "/login/oauth/access_token"
}

func requestJSON(method, url string, in any, out any) error {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "codexcopilot/0.3.0")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("http %d from %s: %s", res.StatusCode, url, string(data))
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func Load(p paths.Paths) (*Auth, error) {
	data, err := os.ReadFile(p.AuthFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var auth Auth
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, err
	}
	if auth.AccessToken == "" {
		return nil, nil
	}
	return &auth, nil
}

func Save(p paths.Paths, auth Auth) error {
	if err := os.MkdirAll(p.StateDir, 0o700); err != nil {
		return err
	}
	auth.TokenType = "oauth"
	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := p.AuthFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.AuthFile)
}

func Logout(p paths.Paths) (bool, error) {
	if err := os.Remove(p.AuthFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func Login(p paths.Paths, clientIDValue string, enterpriseURL string) (*Auth, error) {
	deviceURL, tokenURL := oauthURLs(enterpriseURL)
	var device struct {
		VerificationURI string `json:"verification_uri"`
		UserCode        string `json:"user_code"`
		DeviceCode      string `json:"device_code"`
		Interval        int    `json:"interval"`
		ExpiresIn       int    `json:"expires_in"`
	}
	if err := requestJSON("POST", deviceURL, map[string]string{
		"client_id": clientID(clientIDValue),
		"scope":     "read:user",
	}, &device); err != nil {
		return nil, err
	}
	if device.Interval <= 0 {
		device.Interval = 5
	}
	if device.ExpiresIn <= 0 {
		device.ExpiresIn = 900
	}
	fmt.Printf("Open %s and enter code: %s\n", device.VerificationURI, device.UserCode)
	fmt.Println("Waiting for GitHub authorization...")
	deadline := time.Now().Add(time.Duration(device.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(time.Duration(device.Interval) * time.Second)
		var token struct {
			AccessToken      string `json:"access_token"`
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
			Interval         int    `json:"interval"`
		}
		err := requestJSON("POST", tokenURL, map[string]string{
			"client_id":   clientID(clientIDValue),
			"device_code": device.DeviceCode,
			"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
		}, &token)
		if err != nil {
			return nil, err
		}
		if token.AccessToken != "" {
			result := Auth{AccessToken: token.AccessToken, EnterpriseURL: normalizeDomain(enterpriseURL)}
			if enterpriseURL == "" {
				result.EnterpriseURL = ""
			}
			if err := Save(p, result); err != nil {
				return nil, err
			}
			return &result, nil
		}
		switch token.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			if token.Interval > 0 {
				device.Interval = token.Interval
			} else {
				device.Interval += 5
			}
			continue
		case "":
			continue
		default:
			if token.ErrorDescription != "" {
				return nil, errors.New(token.ErrorDescription)
			}
			return nil, errors.New(token.Error)
		}
	}
	return nil, errors.New("GitHub device authorization expired")
}
