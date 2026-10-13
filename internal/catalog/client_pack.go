package catalog

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
)

// ResolveClient pins a mod to the installed server release. Paper plugins may
// have a separate client build, which must match that release's version number.
func (c *Client) ResolveClient(ctx context.Context, project, installed, loader, game string) ([]Package, error) {
	if !identifier.MatchString(installed) || !identifier.MatchString(project) {
		return nil, ErrUnavailable
	}
	var v Version
	if err := c.get(ctx, "version/"+installed, &v); err != nil || v.ProjectID != project {
		return nil, ErrUnavailable
	}
	if Compatible(v, loader, game) {
		return c.resolve(ctx, project, v.ID, loader, game, true)
	}
	if v.Number == "" {
		return nil, ErrUnavailable
	}
	l, _ := json.Marshal([]string{loader})
	g, _ := json.Marshal([]string{game})
	q := url.Values{"loaders": {string(l)}, "game_versions": {string(g)}}
	var candidates []Version
	if err := c.get(ctx, "project/"+project+"/version?"+q.Encode(), &candidates); err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		if candidate.ProjectID == project && sameClientRelease(project, v.Number, candidate.Number, loader, game) && Compatible(candidate, loader, game) {
			return c.resolve(ctx, project, candidate.ID, loader, game, true)
		}
	}
	return nil, ErrUnavailable
}

func sameClientRelease(project, serverNumber, clientNumber, loader, game string) bool {
	if serverNumber == clientNumber {
		return true
	}
	// Simple Voice Chat publishes Bukkit and client builds under platform-specific
	// version numbers. Match the addon release exactly; never silently use latest.
	return project == "9eGKb6K1" && strings.HasPrefix(serverNumber, "bukkit-") && clientNumber == loader+"-"+strings.TrimPrefix(serverNumber, "bukkit-")+"+"+game
}
