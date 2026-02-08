package strategy

import (
	"fmt"

	"github.com/derekprior/rbrl/internal/config"
)

// Game represents a single matchup between two teams.
type Game struct {
	Home  string
	Away  string
	Label string // unique identifier like "Game 1"
}

// Strategy generates the list of matchups for a season.
type Strategy interface {
	GenerateMatchups(divisions []config.Division) []Game
}

// Get returns a Strategy by name.
func Get(name string) (Strategy, error) {
	switch name {
	case "division_weighted":
		return &DivisionWeighted{}, nil
	default:
		return nil, fmt.Errorf("unknown strategy: %q", name)
	}
}

// DivisionWeighted generates matchups where intra-division opponents play
// twice and inter-division opponents play once.
type DivisionWeighted struct{}

func (s *DivisionWeighted) GenerateMatchups(divisions []config.Division) []Game {
	var games []Game
	gameNum := 1

	// Intra-division: each pair plays twice (home/away split)
	for _, div := range divisions {
		for i := 0; i < len(div.Teams); i++ {
			for j := i + 1; j < len(div.Teams); j++ {
				games = append(games,
					Game{
						Home:  div.Teams[i],
						Away:  div.Teams[j],
						Label: fmt.Sprintf("Game %d", gameNum),
					},
				)
				gameNum++
				games = append(games,
					Game{
						Home:  div.Teams[j],
						Away:  div.Teams[i],
						Label: fmt.Sprintf("Game %d", gameNum),
					},
				)
				gameNum++
			}
		}
	}

	// Inter-division: each cross-division pair plays once.
	// Alternate home/away to balance across teams.
	if len(divisions) == 2 {
		d0, d1 := divisions[0], divisions[1]
		for i, t0 := range d0.Teams {
			for j, t1 := range d1.Teams {
				home, away := t0, t1
				if (i+j)%2 == 1 {
					home, away = t1, t0
				}
				games = append(games, Game{
					Home:  home,
					Away:  away,
					Label: fmt.Sprintf("Game %d", gameNum),
				})
				gameNum++
			}
		}
	}

	return games
}
