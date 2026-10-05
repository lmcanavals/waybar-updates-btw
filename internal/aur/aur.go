// Package aur provides functions to check for Arch User Repository (AUR) package updates via the AUR RPC API.
package aur

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"

	"github.com/lmcanavals/waybar-updates-btw/internal/alpm"
)

// Package represents a package entry returned by the AUR RPC API.
type Package struct {
	Name    string `json:"Name"`
	Version string `json:"Version"`
}

// Response represents the payload envelope from the AUR RPC API.
type Response struct {
	Results []Package `json:"results"`
}

// Check queries local foreign packages via `pacman -Qm` and checks the AUR RPC API for updates.
// Returns:
//   - updates: formatted as "aur/<pkgname> <oldVer> -> <newVer>"
//   - ok: true if the query succeeded
//   - err: non-nil if pacman or AUR API failed
func Check() ([]string, bool, error) {
	output, err := exec.Command("pacman", "-Qm").Output()
	if err != nil {
		return nil, false, fmt.Errorf("pacman -Qm: %w", err)
	}

	localPackages := make(map[string]string)
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			localPackages[parts[0]] = parts[1]
		}
	}

	if len(localPackages) == 0 {
		return []string{}, true, nil
	}

	packageNames := make([]string, 0, len(localPackages))
	for name := range localPackages {
		packageNames = append(packageNames, name)
	}

	aurPackages, err := queryAPI(packageNames)
	if err != nil {
		return nil, false, fmt.Errorf("query AUR API: %w", err)
	}

	var updates []string
	for _, aurPkg := range aurPackages {
		localVer, ok := localPackages[aurPkg.Name]
		if ok && alpm.Compare(aurPkg.Version, localVer) > 0 {
			updates = append(updates, fmt.Sprintf("aur/%s %s -> %s", aurPkg.Name, localVer, aurPkg.Version))
		}
	}

	return updates, true, nil
}

func queryAPI(packageNames []string) ([]Package, error) {
	if len(packageNames) == 0 {
		return nil, nil
	}
	u, _ := url.Parse("https://aur.archlinux.org/rpc/")
	q := u.Query()
	q.Set("v", "5")
	q.Set("type", "info")
	for _, name := range packageNames {
		q.Add("arg[]", name)
	}
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var aurResponse Response
	err = json.Unmarshal(body, &aurResponse)
	if err != nil {
		return nil, fmt.Errorf("unmarshal AUR response: %w", err)
	}
	return aurResponse.Results, nil
}
