package publicapi

import "math/rand/v2"

// ActorHues is the set Graph Editor draws a new actor's colour from: the eight chromatic hues of
// simulator-ui-kit packages/themes/src/themes/actorHues.ts. red (too close to the error colour) and
// the neutral slate are offered there but never handed out at random, so they are not here either.
var ActorHues = []string{"#875bf7", "#2970ff", "#36bffa", "#17b26a", "#66c61c", "#fac515", "#f38744", "#f63d68"}

// RandomActorColor picks one ActorHues entry uniformly, like the editor's pickActorColor.
func RandomActorColor() string { return ActorHues[rand.IntN(len(ActorHues))] }
