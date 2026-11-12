package app

import "fmt"

const agentNavigationMaximumCells = 262144
const agentCollisionCellPixels = 20 // Native collision-cell world spacing.

type AgentNavigationRequest struct {
	Session int    `json:"session,omitempty"`
	Query   string `json:"query"`
	X       int    `json:"x,omitempty"`
	Y       int    `json:"y,omitempty"`
	Radius  int    `json:"radius,omitempty"`
	Step    int    `json:"step,omitempty"`
}

func (c *Client) AgentNavigation(p AgentNavigationRequest) (any, error) {
	if err := c.AgentAccessError(); err != nil {
		return nil, err
	}
	if c.World == nil || c.World.Scene == nil {
		return nil, fmt.Errorf("map is unavailable")
	}
	if p.X < 0 || p.Y < 0 || p.X > 65535 || p.Y > 65535 {
		return nil, fmt.Errorf("invalid world coordinates")
	}
	switch p.Query {
	case "plan":
		return c.AgentPlan(p.X, p.Y), nil
	case "terrain":
		return map[string]bool{"land": c.AgentLand(p.X, p.Y), "water": c.AgentWater(p.X, p.Y)}, nil
	case "grid":
		if p.Radius < 1 || p.Radius > 100 {
			return nil, fmt.Errorf("radius must be 1..100 collision cells")
		}
		x, y, cols, rows, cells := c.AgentWalkGrid(p.Radius)
		return map[string]any{"originX": x, "originY": y, "cols": cols, "rows": rows, "cellSize": agentCollisionCellPixels, "cells": cells}, nil
	case "overview":
		if p.Step < 1 || p.Step > 64 {
			return nil, fmt.Errorf("step must be 1..64 collision cells")
		}
		w, h := c.AgentMapSize()
		stride := p.Step * agentCollisionCellPixels
		if ((w+stride-1)/stride)*((h+stride-1)/stride) > agentNavigationMaximumCells {
			return nil, fmt.Errorf("overview too large; choose a larger step")
		}
		cols, rows, cells := c.AgentMapGrid(p.Step)
		return map[string]any{"cols": cols, "rows": rows, "step": p.Step, "cellSize": agentCollisionCellPixels, "cells": cells}, nil
	}
	return nil, fmt.Errorf("unknown navigation query")
}
