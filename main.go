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
	var (
		interval, intervalSync                                   int
		skipAur, rawOutput, noColor                              bool
		colorMajor, colorMinor, colorPatch, colorPre, colorOther string
	)
	flag.IntVar(&interval, "interval", 10, "Set the interval between updates in seconds.")
	flag.IntVar(&intervalSync, "interval-sync", 300, "Set the interval between sync updates in seconds.")
	flag.BoolVar(&skipAur, "skip-aur", false, "Skips checking for AUR updates.")
	flag.BoolVar(&rawOutput, "raw-output", false, "Disables formating tooltip text into columns.")
	flag.BoolVar(&noColor, "no-color", false, "Disables coloring packages by version category.")
	flag.StringVar(&colorMajor, "color-major", "f7768e", "Color used for major version update, ignored if -no-color is present.")
	flag.StringVar(&colorMinor, "color-minor", "ff9e64", "Color used for minor version update, ignored if -no-color is present.")
	flag.StringVar(&colorPatch, "color-patch", "e0af68", "Color used for patch update, ignored if -no-color is present.")
	flag.StringVar(&colorPre, "color-pre", "9ece6a", "Color used for pre update, ignored if -no-color is present.")
	flag.StringVar(&colorOther, "color-other", "7dcfff", "Color used for some other update, ignored if -no-color is present.")
	colors := []string{colorMajor, colorMinor, colorPatch, colorPre, colorOther}

	flag.Parse()

	if interval == 0 || intervalSync < 10 || intervalSync < interval {
		fmt.Println("`interval` and `interval-sync` must be greater than 0 and 9 respectively and `interval-sync` must be greater or equal to `interval`.")
		os.Exit(1)
	}

	encoder := json.NewEncoder(os.Stdout)
	result := &Result{"0", "Checking for updates...", "has-updates", "has-updates"}
	updates := make([]string, 0, 50)
	chPacmanUpdates := make(chan []string)
	chAurUpdates := make(chan []string)
	updateOnIter := intervalSync / interval
	intervalDuration := time.Duration(interval) * time.Second
	go checkUpdates(chPacmanUpdates, updateOnIter, intervalDuration)
	if !skipAur {
		go checkAurUpdates(chAurUpdates, updateOnIter, intervalDuration)
	}
	var updatesAur, updatesPac []string

	for {
		if encoder.Encode(result) != nil {
			os.Exit(2)
		}
		select {
		case tempPac := <-chPacmanUpdates:
			if tempPac != nil {
				updatesPac = tempPac
			}
		case tempAur := <-chAurUpdates:
			if tempAur != nil {
				updatesAur = tempAur
			}
		}
		updates = updates[:0]
		updates = append(updates, updatesPac...)
		updates = append(updates, updatesAur...)

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

func addFormat(updates, colors []string, rawOutput, noColor bool) {
	allParts := make([][]string, len(updates))
	maxNameLen := 0
	maxVersionLen := 0
	var formatStr strings.Builder
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
		fmt.Fprint(&formatStr, "<span font-family='monospace'")
		if !noColor {
			category := parseVersion(part[1], part[3])
			fmt.Fprintf(&formatStr, " color='#%s'", colors[category])
		}
		if rawOutput {
			fmt.Fprintf(&formatStr, ">%%s %%s -> %%s</span>")
		} else {
			fmt.Fprintf(&formatStr, ">%%-%ds %%-%ds -> %%s</span>", maxNameLen, maxVersionLen)
		}
		updates[i] = fmt.Sprintf(formatStr.String(), part[0], part[1], part[3])
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

func checkUpdates(chUpdates chan<- []string, updateOnIter int, intervalDuration time.Duration) {
	var cmd *exec.Cmd
	iter := updateOnIter
	for {
		if iter == updateOnIter {
			cmd = exec.Command("checkupdates", "--nocolor")
			iter = 0
		} else {
			cmd = exec.Command("checkupdates", "--nosync", "--nocolor")
		}
		iter++
		output, err := cmd.Output()
		if err != nil {
			if exiterr, ok := err.(*exec.ExitError); ok && exiterr.ExitCode() == 2 {
				chUpdates <- nil
			} else {
				chUpdates <- nil
				// chUpdates <- []string{fmt.Sprintf("Unexpected: %v", err)}
			}
		} else {
			chUpdates <- strings.Split(strings.TrimSpace(string(output)), "\n")
		}
		time.Sleep(intervalDuration)
	}
}

func checkAurUpdates(chUpdates chan<- []string, updateOnIter int, intervalDuration time.Duration) {
	iter := updateOnIter
	firstCall := true
	for {
		if firstCall {
			firstCall = false
		} else {
			time.Sleep(intervalDuration)
		}
		if iter != updateOnIter {
			iter++
			chUpdates <- nil
			continue
		}
		iter = 1
		output, err := exec.Command("pacman", "-Qm").Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running pacman -Qm: %v\n", err)
			chUpdates <- []string{fmt.Sprintf("Error running pacman -Qm: %v", err)}
		}

		if len(output) == 0 {
			chUpdates <- []string{"Nothing from aur installed"}
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
			chUpdates <- []string{fmt.Sprintf("Error querying AUR API: %v", err)}
		}

		var updates []string
		for _, aurPkg := range aurPackages {
			if aurPkg.Version != localPackages[aurPkg.Name] {
				updates = append(updates, fmt.Sprintf("aur/%s %s -> %s", aurPkg.Name, localPackages[aurPkg.Name], aurPkg.Version))
			}
		}

		chUpdates <- updates
	}
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
