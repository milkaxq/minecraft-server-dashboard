package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

var ErrPreviewClient = errors.New("matching client build is a prerelease")

// ResolveClient pins a mod to the installed server release. Paper plugins may
// have a separate client build, which must match that release's version number.
func (c *Client) ResolveClient(ctx context.Context, project, installed, loader, game string) ([]Package, error) {
	return c.ResolveClientWithBeta(ctx, project, installed, loader, game, false)
}

func (c *Client) ResolveClientWithBeta(ctx context.Context, project, installed, loader, game string, allowBeta bool) ([]Package, error) {
	if !identifier.MatchString(installed) || !identifier.MatchString(project) {
		return nil, ErrUnavailable
	}
	var v Version
	if err := c.get(ctx, "version/"+installed, &v); err != nil || v.ProjectID != project {
		return nil, ErrUnavailable
	}
	if clientCompatible(v, loader, game, allowBeta) {
		return c.resolveWithBeta(ctx, project, v.ID, loader, game, true, allowBeta)
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
	preview := false
	for _, candidate := range candidates {
		if candidate.ProjectID == project && sameClientRelease(project, v.Number, candidate.Number, loader, game) && candidate.Type == "beta" {
			stable := candidate
			stable.Type = "release"
			preview = preview || Compatible(stable, loader, game)
		}
		if candidate.ProjectID == project && sameClientRelease(project, v.Number, candidate.Number, loader, game) && clientCompatible(candidate, loader, game, allowBeta) {
			return c.resolveWithBeta(ctx, project, candidate.ID, loader, game, true, allowBeta)
		}
	}
	if preview {
		return nil, ErrPreviewClient
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

// Beta acceptance only relaxes the release channel, never loader or game matching.
func clientCompatible(v Version, loader, game string, allowBeta bool) bool {
	if allowBeta && v.Type == "beta" {
		v.Type = "release"
	}
	return Compatible(v, loader, game)
}
