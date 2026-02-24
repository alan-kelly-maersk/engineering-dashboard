package models

import "testing"

func TestRepoDependencySummary_TotalDeps(t *testing.T) {
	tests := []struct {
		name   string
		goMods []GoModInfo
		want   int
	}{
		{"no go mods", nil, 0},
		{"empty go mods", []GoModInfo{}, 0},
		{"single go mod", []GoModInfo{{TotalDeps: 5}}, 5},
		{"multiple go mods", []GoModInfo{{TotalDeps: 3}, {TotalDeps: 7}}, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepoDependencySummary{GoMods: tt.goMods}
			if got := r.TotalDeps(); got != tt.want {
				t.Errorf("TotalDeps() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRepoDependencySummary_OutdatedDeps(t *testing.T) {
	tests := []struct {
		name   string
		goMods []GoModInfo
		want   int
	}{
		{"no go mods", nil, 0},
		{"no outdated", []GoModInfo{{OutdatedDeps: 0}}, 0},
		{"some outdated", []GoModInfo{{OutdatedDeps: 2}, {OutdatedDeps: 3}}, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepoDependencySummary{GoMods: tt.goMods}
			if got := r.OutdatedDeps(); got != tt.want {
				t.Errorf("OutdatedDeps() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRepoDependencySummary_HasOutdatedGoVersion(t *testing.T) {
	tests := []struct {
		name   string
		goMods []GoModInfo
		want   bool
	}{
		{"no go mods", nil, false},
		{"not outdated", []GoModInfo{{GoVersionOutdated: false}}, false},
		{"one outdated", []GoModInfo{{GoVersionOutdated: true}}, true},
		{"mixed", []GoModInfo{{GoVersionOutdated: false}, {GoVersionOutdated: true}}, true},
		{"none outdated multi", []GoModInfo{{GoVersionOutdated: false}, {GoVersionOutdated: false}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepoDependencySummary{GoMods: tt.goMods}
			if got := r.HasOutdatedGoVersion(); got != tt.want {
				t.Errorf("HasOutdatedGoVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}
