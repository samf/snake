package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const defaultServer = "https://TheSnakeCan.samf.work"

type LoginCmd struct {
	Server string `help:"Snake Can server URL." default:"https://TheSnakeCan.samf.work" env:"SNAKE_SERVER"`
}

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type tokenPollResponse struct {
	Token string `json:"token"`
	Error string `json:"error"`
}

type cansAPIResponse struct {
	Cans []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"cans"`
	PreferredCan string `json:"preferredCan"`
}

func (l *LoginCmd) Run() error {
	// 1. Request a device code.
	resp, err := http.PostForm(l.Server+"/auth/device", nil)
	if err != nil {
		return fmt.Errorf("could not reach server: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var dr deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		return fmt.Errorf("unexpected server response: %w", err)
	}

	// 2. Open the browser and prompt the user.
	directURL := dr.VerificationURI + "?code=" + url.QueryEscape(dr.UserCode)
	if err := openBrowser(directURL); err != nil {
		fmt.Printf("\nTo authenticate, visit:\n  %s\n\n", dr.VerificationURI)
		fmt.Printf("And enter the code:\n  %s\n\n", dr.UserCode)
		fmt.Printf("Or go directly:\n  %s\n\n", directURL)
	} else {
		fmt.Printf("\nOpening browser to:\n  %s\n\n", directURL)
	}
	fmt.Println("Waiting for authentication…")

	// 3. Poll for the token.
	interval := time.Duration(dr.Interval) * time.Second
	deadline := time.Now().Add(time.Duration(dr.ExpiresIn) * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(interval)

		token, pending, err := pollToken(l.Server, dr.DeviceCode)
		if err != nil {
			return err
		}
		if pending {
			continue
		}

		// 4. Save the token.
		if err := saveConfig(Config{Server: l.Server, Token: token}); err != nil {
			return fmt.Errorf("could not save config: %w", err)
		}
		fmt.Println("\nAuthenticated!")

		// 5. Fetch the user's cans and auto-configure can_id.
		if err := fetchAndSetCan(l.Server, token); err != nil {
			fmt.Printf("Note: could not fetch cans: %v\n", err)
		}

		path, _ := configPath()
		fmt.Printf("\nConfig saved to %s\n", path)
		return nil
	}

	return fmt.Errorf("authentication timed out — run 'snake login' to try again")
}

func pollToken(server, deviceCode string) (token string, pending bool, err error) {
	resp, err := http.PostForm(server+"/auth/token", url.Values{"device_code": {deviceCode}})
	if err != nil {
		return "", true, nil // treat network errors as transient
	}
	defer resp.Body.Close()

	var tr tokenPollResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", true, nil
	}
	switch tr.Error {
	case "":
		return tr.Token, false, nil
	case "authorization_pending":
		return "", true, nil
	case "expired_token":
		return "", false, fmt.Errorf("code expired — run 'snake login' to try again")
	case "access_denied":
		return "", false, fmt.Errorf("access denied")
	default:
		return "", true, nil
	}
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// resolveCanID fetches the user's cans and returns the ID and name of the can to use.
// It picks the preferred can (if set) or the only can. Returns an error if the choice
// is ambiguous or no cans exist.
func resolveCanID(server, token string) (id, name string, err error) {
	req, err := http.NewRequest(http.MethodGet, server+"/cans", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var cr cansAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", "", err
	}

	canName := func(c struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}) string {
		if c.Name == "" {
			return "untitled"
		}
		return c.Name
	}

	switch len(cr.Cans) {
	case 0:
		return "", "", fmt.Errorf("you have no cans yet — create one at %s/cans", server)
	case 1:
		return cr.Cans[0].ID, canName(cr.Cans[0]), nil
	default:
		if cr.PreferredCan != "" {
			for _, can := range cr.Cans {
				if can.ID == cr.PreferredCan {
					return can.ID, canName(can), nil
				}
			}
		}
		var sb strings.Builder
		sb.WriteString("you have multiple cans; set a preferred can at the web UI or specify --can:\n")
		for _, can := range cr.Cans {
			fmt.Fprintf(&sb, "  %s  %s\n", can.ID, canName(can))
		}
		return "", "", fmt.Errorf("%s", strings.TrimRight(sb.String(), "\n"))
	}
}

func fetchAndSetCan(server, token string) error {
	id, name, err := resolveCanID(server, token)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	if err := saveConfig(Config{CanID: id}); err != nil {
		return err
	}
	fmt.Printf("Using can: %s (%s)\n", name, id)
	return nil
}
