package config

import (
	"os"
	"path/filepath"
	"strconv"

	gosshconfig "github.com/kevinburke/ssh_config"
)

// SSHEntry is a parsed entry from ~/.ssh/config.
type SSHEntry struct {
	Alias    string
	Hostname string
	User     string
	Port     int
}

// ParseSSHConfig reads ~/.ssh/config and returns all non-wildcard host entries.
func ParseSSHConfig() ([]SSHEntry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(home, ".ssh", "config"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg, err := gosshconfig.Decode(f)
	if err != nil {
		return nil, err
	}

	var entries []SSHEntry
	for _, host := range cfg.Hosts {
		// Skip wildcard entries
		if len(host.Patterns) == 0 {
			continue
		}
		for _, pat := range host.Patterns {
			alias := pat.String()
			if alias == "*" {
				continue
			}
			hostname := gosshconfig.Get(alias, "Hostname")
			if hostname == "" {
				hostname = alias
			}
			user := gosshconfig.Get(alias, "User")
			portStr := gosshconfig.Get(alias, "Port")
			port := 22
			if portStr != "" {
				if p, err := strconv.Atoi(portStr); err == nil {
					port = p
				}
			}
			entries = append(entries, SSHEntry{
				Alias:    alias,
				Hostname: hostname,
				User:     user,
				Port:     port,
			})
		}
	}
	return entries, nil
}

// HostValidationResult holds the outcome of validating a single project host.
type HostValidationResult struct {
	Host       Host
	Resolved   bool
	ResolvedAs string
	Warning    string
}

// ValidateProjectHosts checks each host in a project against ~/.ssh/config.
// Strategy:
//  1. Exact alias match → use it.
//  2. Soft hostname match → use it (handles renames).
//  3. Unresolvable → Resolved = false.
func ValidateProjectHosts(proj Project) []HostValidationResult {
	entries, _ := ParseSSHConfig()

	// Build lookup maps.
	byAlias := make(map[string]SSHEntry)
	byHostname := make(map[string]SSHEntry)
	for _, e := range entries {
		byAlias[e.Alias] = e
		byHostname[e.Hostname] = e
	}

	results := make([]HostValidationResult, 0, len(proj.Hosts))
	for _, h := range proj.Hosts {
		r := HostValidationResult{Host: h}

		if e, ok := byAlias[h.Name]; ok {
			r.Resolved = true
			r.ResolvedAs = e.Alias
		} else if e, ok := byHostname[h.Hostname]; ok {
			r.Resolved = true
			r.ResolvedAs = e.Alias
			r.Warning = "SSH alias changed; matched by hostname " + h.Hostname
		} else {
			r.Resolved = false
			r.Warning = "host " + h.Name + " not found in ~/.ssh/config"
		}
		results = append(results, r)
	}
	return results
}
