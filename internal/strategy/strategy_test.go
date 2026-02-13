package strategy

import (
	"testing"

	"github.com/derekprior/rbrl/internal/config"
)

func testDivisions() []config.Division {
	return []config.Division{
		{Name: "American", Teams: []string{"Angels", "Astros", "Athletics", "Mariners", "Royals"}},
		{Name: "National", Teams: []string{"Cubs", "Padres", "Phillies", "Pirates", "Marlins"}},
	}
}

func TestDivisionWeightedMatchups(t *testing.T) {
	s := &DivisionWeighted{}
	divs := testDivisions()
	games := s.GenerateMatchups(divs)

	t.Run("total game count", func(t *testing.T) {
		// 5 teams × 13 games / 2 = 65 total games
		// Intra-division: C(5,2) × 2 per division × 2 divisions = 10×2×2 = 40... no
		// Intra per division: C(5,2)=10 pairs × 2 games = 20 games. × 2 divisions = 40
		// Inter: 5×5=25 pairs × 1 game = 25 games
		// Total: 40 + 25 = 65
		if len(games) != 65 {
			t.Errorf("total games = %d, want 65", len(games))
		}
	})

	t.Run("each team plays 13 games", func(t *testing.T) {
		counts := make(map[string]int)
		for _, g := range games {
			counts[g.Home]++
			counts[g.Away]++
		}
		for _, div := range divs {
			for _, team := range div.Teams {
				if counts[team] != 13 {
					t.Errorf("%s plays %d games, want 13", team, counts[team])
				}
			}
		}
	})

	t.Run("intra-division teams play each other twice", func(t *testing.T) {
		type pair struct{ a, b string }
		matchups := make(map[pair]int)
		for _, g := range games {
			a, b := g.Home, g.Away
			if a > b {
				a, b = b, a
			}
			matchups[pair{a, b}]++
		}

		intraPairs := []pair{
			{"Angels", "Astros"}, {"Angels", "Athletics"}, {"Angels", "Mariners"}, {"Angels", "Royals"},
			{"Astros", "Athletics"}, {"Astros", "Mariners"}, {"Astros", "Royals"},
			{"Athletics", "Mariners"}, {"Athletics", "Royals"},
			{"Mariners", "Royals"},
		}
		for _, p := range intraPairs {
			if matchups[p] != 2 {
				t.Errorf("%s vs %s = %d games, want 2", p.a, p.b, matchups[p])
			}
		}
	})

	t.Run("inter-division teams play each other once", func(t *testing.T) {
		type pair struct{ a, b string }
		matchups := make(map[pair]int)
		for _, g := range games {
			a, b := g.Home, g.Away
			if a > b {
				a, b = b, a
			}
			matchups[pair{a, b}]++
		}

		for _, at := range testDivisions()[0].Teams {
			for _, nt := range testDivisions()[1].Teams {
				a, b := at, nt
				if a > b {
					a, b = b, a
				}
				if matchups[pair{a, b}] != 1 {
					t.Errorf("%s vs %s = %d games, want 1", a, b, matchups[pair{a, b}])
				}
			}
		}
	})

	t.Run("home/away roughly balanced", func(t *testing.T) {
		home := make(map[string]int)
		away := make(map[string]int)
		for _, g := range games {
			home[g.Home]++
			away[g.Away]++
		}
		for _, div := range divs {
			for _, team := range div.Teams {
				diff := home[team] - away[team]
				if diff < -2 || diff > 2 {
					t.Errorf("%s home/away imbalance: %d home, %d away", team, home[team], away[team])
				}
			}
		}
	})

	t.Run("each game has a label", func(t *testing.T) {
		seen := make(map[string]bool)
		for _, g := range games {
			if g.Label == "" {
				t.Error("game has empty label")
			}
			if seen[g.Label] {
				t.Errorf("duplicate label: %s", g.Label)
			}
			seen[g.Label] = true
		}
	})
}

func TestDivisionWeightedSmall(t *testing.T) {
	s := &DivisionWeighted{}
	divs := []config.Division{
		{Name: "A", Teams: []string{"T1", "T2", "T3"}},
		{Name: "B", Teams: []string{"T4", "T5", "T6"}},
	}
	games := s.GenerateMatchups(divs)

	// Intra: C(3,2)=3 pairs × 2 = 6 per division × 2 = 12
	// Inter: 3×3=9 pairs × 1 = 9
	// Total: 21, each team plays 2×2 + 3 = 7 games
	if len(games) != 21 {
		t.Errorf("total games = %d, want 21", len(games))
	}

	counts := make(map[string]int)
	for _, g := range games {
		counts[g.Home]++
		counts[g.Away]++
	}
	for _, team := range []string{"T1", "T2", "T3", "T4", "T5", "T6"} {
		if counts[team] != 7 {
			t.Errorf("%s plays %d games, want 7", team, counts[team])
		}
	}
}
