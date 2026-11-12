package publicapi

import (
	"context"
	"errors"
	"strings"
)

// ReactionStats preserves the reactionStatRes dynamic type->integer map from public
// reaction/stats/get. Missing types stay absent; {} is distinct from a failed read.
type ReactionStats map[string]int64

// reactionStatsRequest follows public reaction/stats/get; workspace is host-owned.
type reactionStatsRequest struct {
	AccID       string `json:"accId"`
	RootActorID string `json:"rootActorId"`
}

func (c *Client) ReactionReadsEnabled() bool { return c.config.Profile == "reactions-read" }

// ReactionActorBound is host configuration, never a delegated permission grant.
func (c *Client) ReactionActorBound() bool { return c.ReactionReadsEnabled() && c.config.ActorID != "" }

func (c *Client) GetReactionStats(ctx context.Context, actorID string) (ReactionStats, error) {
	if !c.ReactionReadsEnabled() {
		return nil, errors.New("reactions-read profile is disabled")
	}
	if c.ReactionActorBound() {
		if actorID != "" {
			return nil, errors.New("actorId is host-bound; omit tool arguments")
		}
		actorID = c.config.ActorID
	}
	if strings.TrimSpace(actorID) == "" || len(actorID) > 255 {
		return nil, errors.New("actorId required, maximum 255 bytes")
	}
	// Pointers reject null counters instead of silently converting null to zero.
	wire, err := call[reactionStatsRequest, map[string]*int64](ctx, c, "reaction/stats/get", reactionStatsRequest{AccID: c.workspace(), RootActorID: actorID})
	if err != nil {
		return nil, err
	}
	result := make(ReactionStats, len(wire))
	for name, count := range wire {
		if name == "" || count == nil || *count < 0 {
			return nil, errors.New("invalid reaction counter result")
		}
		result[name] = *count
	}
	return result, nil
}
