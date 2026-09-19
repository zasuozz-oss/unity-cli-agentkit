package filter

import (
	"encoding/json"
	"strconv"
	"strings"
)

type statusInstance struct {
	ProjectName     string `json:"projectName"`
	ProjectPath     string `json:"projectPath"`
	PID             int    `json:"pid"`
	IsRunning       bool   `json:"isRunning"`
	PipelineVersion string `json:"pipelineVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	PipelineServer  struct {
		Port        int  `json:"port"`
		IsReachable bool `json:"isReachable"`
	} `json:"pipelineServer"`
	SafeMode *struct {
		Detected bool `json:"detected"`
	} `json:"safeMode"`
}

// ProjectPath returns the project of the single reachable instance in a
// `unity pipeline list --json` payload, or "" when that is ambiguous — with
// zero or several editors up, there is no one project to speak for.
func ProjectPath(data []byte) string {
	var d struct {
		Instances []statusInstance `json:"instances"`
	}
	if json.Unmarshal(data, &d) != nil {
		return ""
	}
	found := ""
	for _, in := range d.Instances {
		if !in.PipelineServer.IsReachable {
			continue
		}
		if found != "" {
			return ""
		}
		found = in.ProjectPath
	}
	return found
}

// Status compacts `unity pipeline list --json` data to one line per editor
// instance. Unexpected input passes through unchanged (loss-safe).
func Status(data []byte) []byte {
	var d struct {
		Instances     *[]statusInstance `json:"instances"`
		LatestVersion string            `json:"latestVersion"`
	}
	if err := json.Unmarshal(data, &d); err != nil || d.Instances == nil {
		return data
	}
	if len(*d.Instances) == 0 {
		return []byte("no editor instances (is Unity open with com.unity.pipeline installed?)\n")
	}
	var b strings.Builder
	for _, in := range *d.Instances {
		state := "UNREACHABLE"
		if in.PipelineServer.IsReachable {
			state = "reachable"
		}
		// Safe Mode is why the server is down, not a second state — without
		// this the agent reads UNREACHABLE as "no Editor" and blind-edits.
		if in.SafeMode != nil && in.SafeMode.Detected {
			state = "SAFE MODE (compile errors — fix them, then restart Unity)"
		}
		// An outdated package stopped being cosmetic at CLI 1.0.0-beta.9: it
		// sends command lines for the package to bind, and anything older than
		// 0.6.0-exp.1 cannot parse them, so every exec verb fails. status is
		// where the README sends you to check that pairing. The flag reflects
		// the version resolved on disk, so it clears as soon as the upgrade
		// lands — an Editor that has not domain-reloaded the new assembly yet
		// still fails, and only a reload fixes that.
		version := in.PipelineVersion
		if in.UpdateAvailable {
			version += " OUTDATED"
			// `unity pipeline upgrade` is the obvious command and it is broken
			// on CLI 1.0.0-beta.9: it answers alreadyLatest:true while sitting
			// on an older version, even though `pipeline list-versions` marks
			// the newer one Latest. Name the install form, which works — and
			// name the version, since it needs one.
			if d.LatestVersion != "" {
				version += "→" + d.LatestVersion +
					" (unity pipeline install --package-version " + d.LatestVersion + ")"
			} else {
				version += " (unity pipeline install --package-version <latest>)"
			}
		}
		b.WriteString(in.ProjectName +
			"  pipeline " + version +
			"  pid " + strconv.Itoa(in.PID) +
			"  port " + strconv.Itoa(in.PipelineServer.Port) +
			"  " + state + "\n")
	}
	return []byte(b.String())
}
