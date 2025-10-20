package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Result struct {
	Text    string `json:"text"`
	Tooltip string `json:"tooltip"`
	Class   string `json:"class"`
	Alt     string `json:"alt"`
}

type AurResponse struct {
	Results []AurPackage `json:"results"`
}

type AurPackage struct {
	Name    string `json:"Name"`
	Version string `json:"Version"`
}

func main() {
	interval := *flag.Int("interval", 10, "Set the interval between updates in seconds.")
	intervalSync := *flag.Int("interval-sync", 600, "Set the interval between sync updates in seconds.")
	skipAur := *flag.Bool("skip-aur", false, "Skips checking for AUR updates.")
	rawOutput := *flag.Bool("raw-output", false, "Disables formating tooltip text into columns.")
	noColor := *flag.Bool("no-color", false, "Disables coloring packages by version category.")
	colors := []string{
		*flag.String("color-major", "f7768e", "Color used for major version update, ignored if -no-color is present."),
		*flag.String("color-minor", "ff9e64", "Color used for minor version update, ignored if -no-color is present."),
		*flag.String("color-patch", "e0af68", "Color used for patch update, ignored if -no-color is present."),
		*flag.String("color-pre", "9ece6a", "Color used for pre update, ignored if -no-color is present."),
		*flag.String("color-other", "7dcfff", "Color used for some other update, ignored if -no-color is present."),
	}

	flag.Parse()

	if interval == 0 || intervalSync < 10 || intervalSync < interval {
		fmt.Println("`interval` and `interval-sync` must be greater than 0 and 9 respectively and `interval-sync` must be greater or equal to `interval`.")
		os.Exit(1)
	}

	encoder := json.NewEncoder(os.Stdout)
	result := &Result{"0", "Checking for updates...", "has-updates", "has-updates"}
	chanUpdates := make(chan []string)
	go getUpdates(
		chanUpdates,
		intervalSync/interval,
		time.Duration(interval)*time.Second,
		skipAur,
	)

	for {
		if encoder.Encode(result) != nil {
			os.Exit(2)
		}
		updates := <-chanUpdates
		if len(updates) == 0 {
			result.Text = ""
			result.Tooltip = "All packages are up to date"
			result.Class = "updated"
			result.Alt = "updated"
			continue
		}
		if !rawOutput || !noColor {
			addFormat(updates, colors, rawOutput, noColor)
		}
		result.Text = fmt.Sprintf("%d", len(updates))
		result.Tooltip = strings.Join(updates, "\n")
		result.Class = "has-updates"
		result.Alt = "has-updates"
	}
}

func getUpdates(ch chan<- []string, updateOnIter int, intervalDuration time.Duration, skipAur bool) {
	updates := make([]string, 0, 50)
	var updatesPac, updatesAur []string
	iter := updateOnIter
	for {
		if iter == updateOnIter {
			updatesPac = checkUpdates(true)
			if !skipAur {
				updatesAur = checkAurUpdates()
			}
			iter = 0
		} else {
			updatesPac = checkUpdates(false)
		}

		updates = updates[:0]
		updates = append(updates, updatesPac...)
		if !skipAur {
			updates = append(updates, updatesAur...)
		}
		ch <- updates
		time.Sleep(intervalDuration)
		iter++
	}
}

func addFormat(updates, colors []string, rawOutput, noColor bool) {
	allParts := make([][]string, len(updates))
	maxNameLen := 0
	maxVersionLen := 0
	for i, line := range updates {
		allParts[i] = strings.Fields(line)
		if len(allParts[i]) != 4 {
			continue
		}
		maxNameLen = max(maxNameLen, len(allParts[i][0]))
		maxVersionLen = max(maxVersionLen, len(allParts[i][1]))
	}
	for i, part := range allParts {
		if len(part) != 4 {
			continue
		}
		if noColor {
			formatStr := fmt.Sprintf("<span font-family='monospace'>%%-%ds %%-%ds -> %%s</span>", maxNameLen, maxVersionLen)
			updates[i] = fmt.Sprintf(formatStr, part[0], part[1], part[3])
			continue
		}
		category := parseVersion(part[1], part[3])
		if rawOutput {
			formatStr := fmt.Sprintf("<span font-family='monospace' color='#%s'>%%s %%s -> %%s</span>", colors[category])
			updates[i] = fmt.Sprintf(formatStr, part[0], part[1], part[3])
			continue
		}
		formatStr := fmt.Sprintf("<span font-family='monospace' color='#%s'>%%-%ds %%-%ds -> %%s</span>", colors[category], maxNameLen, maxVersionLen)
		updates[i] = fmt.Sprintf(formatStr, part[0], part[1], part[3])
	}
}

func parseVersion(oldVersion, newVersion string) int {
	dotCounter := 0
	maxLen := max(len(oldVersion), len(newVersion))
	for i := range maxLen {
		if newVersion[i] == '.' || newVersion[i] == '-' {
			dotCounter++
		}
		if newVersion[i] != oldVersion[i] {
			break
		}
	}
	return dotCounter
}

func checkUpdates(sync bool) []string {
	var cmd *exec.Cmd
	if sync {
		cmd = exec.Command("checkupdates", "--nocolor")
	} else {
		cmd = exec.Command("checkupdates", "--nosync", "--nocolor")
	}
	output, err := cmd.Output()
	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok && exiterr.ExitCode() == 2 {
			return []string{}
		} else {
			return []string{fmt.Sprintf("cmd.Wait: %v", err)}
		}
	}

	return strings.Split(strings.TrimSpace(string(output)), "\n")
}

func checkAurUpdates() []string {
	output, err := exec.Command("pacman", "-Qm").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running pacman -Qm: %v\n", err)
		return []string{fmt.Sprintf("Error running pacman -Qm: %v", err)}
	}

	if len(output) == 0 {
		return []string{"Nothing from aur installed"}
	}

	localPackages := make(map[string]string)
	for line := range strings.SplitSeq(string(output), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			localPackages[parts[0]] = parts[1]
		}
	}

	packageNames := make([]string, 0, len(localPackages))
	for name := range localPackages {
		packageNames = append(packageNames, name)
	}

	aurPackages, err := queryAurAPI(packageNames)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error querying AUR API: %v\n", err)
		return []string{fmt.Sprintf("Error querying AUR API: %v", err)}
	}

	var updates []string
	for _, aurPkg := range aurPackages {
		if aurPkg.Version != localPackages[aurPkg.Name] {
			updates = append(updates, fmt.Sprintf("aur/%s %s -> %s", aurPkg.Name, localPackages[aurPkg.Name], aurPkg.Version))
		}
	}

	return updates
}

func queryAurAPI(packageNames []string) ([]AurPackage, error) {
	var url strings.Builder
	url.WriteString("https://aur.archlinux.org/rpc/?v=5&type=info")
	for _, name := range packageNames {
		fmt.Fprintf(&url, "&arg[]=%s", name)
	}

	resp, err := http.Get(url.String())
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AUR API returned status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var aurResponse AurResponse
	if err := json.Unmarshal(body, &aurResponse); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return aurResponse.Results, nil
}
