package commands

import (
	"testing"
)

func Test_expandTagShortcuts(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantExpr     string
		wantTagExprs []string
	}{
		{
			name:         "single include tag",
			input:        "+env",
			wantExpr:     "",
			wantTagExprs: []string{`"env" in tags`},
		},
		{
			name:         "single exclude tag",
			input:        "!brew",
			wantExpr:     "",
			wantTagExprs: []string{`not ("brew" in tags)`},
		},
		{
			name:         "mixed include and exclude tags",
			input:        "+env !brew",
			wantExpr:     "",
			wantTagExprs: []string{`"env" in tags`, `not ("brew" in tags)`},
		},
		{
			name:         "tags with expression",
			input:        `+env name == "test"`,
			wantExpr:     `name == "test"`,
			wantTagExprs: []string{`"env" in tags`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotExpr, gotTagExprs := expandTagShortcuts(tt.input)
			if gotExpr != tt.wantExpr {
				t.Errorf("expandTagShortcuts() expr = %v, want %v", gotExpr, tt.wantExpr)
			}
			if len(gotTagExprs) != len(tt.wantTagExprs) {
				t.Errorf("expandTagShortcuts() tagExprs length = %v, want %v", len(gotTagExprs), len(tt.wantTagExprs))
				return
			}
			for i, expr := range gotTagExprs {
				if expr != tt.wantTagExprs[i] {
					t.Errorf("expandTagShortcuts() tagExprs[%d] = %v, want %v", i, expr, tt.wantTagExprs[i])
				}
			}
		})
	}
}

func Test_expandMacros(t *testing.T) {
	macros := map[string]string{
		"personal": `"home" in tags`,
		"work":     `"office" in tags || "remote" in tags`,
	}

	tests := []struct {
		name    string
		input   string
		macros  map[string]string
		want    string
		wantErr bool
	}{
		{
			name:    "single macro",
			input:   "@personal",
			macros:  macros,
			want:    `("home" in tags)`,
			wantErr: false,
		},
		{
			name:    "macro in expression",
			input:   `@personal && name == "test"`,
			macros:  macros,
			want:    `("home" in tags) && name == "test"`,
			wantErr: false,
		},
		{
			name:    "undefined macro",
			input:   "@undefined",
			macros:  macros,
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandMacros(tt.input, tt.macros)
			if (err != nil) != tt.wantErr {
				t.Errorf("expandMacros() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("expandMacros() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_compileExpr(t *testing.T) {
	macros := map[string]string{
		"personal": `"home" in tags`,
		"work":     `"office" in tags`,
	}

	tests := []struct {
		name             string
		input            string
		macros           map[string]string
		enableExpansions bool
		wantErr          bool
	}{
		{
			name:             "simple expression",
			input:            `name == "test"`,
			macros:           macros,
			enableExpansions: true,
			wantErr:          false,
		},
		{
			name:             "tag shortcuts only",
			input:            "+env !brew",
			macros:           macros,
			enableExpansions: true,
			wantErr:          false,
		},
		{
			name:             "macro expansion",
			input:            "@personal",
			macros:           macros,
			enableExpansions: true,
			wantErr:          false,
		},
		{
			name:             "combined shortcuts and macros",
			input:            "@personal +env !brew",
			macros:           macros,
			enableExpansions: true,
			wantErr:          false,
		},
		{
			name:             "invalid expression syntax",
			input:            "invalid syntax @@",
			macros:           macros,
			enableExpansions: true,
			wantErr:          true,
		},
		{
			name:             "expansions disabled - macro causes error",
			input:            "@personal",
			macros:           macros,
			enableExpansions: false,
			wantErr:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := compileExpr(tt.input, tt.macros, tt.enableExpansions)
			if (err != nil) != tt.wantErr {
				t.Errorf("compileExpr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && program == nil {
				t.Error("compileExpr() returned nil program without error")
			}
		})
	}
}

func Test_evalCompiledExpr(t *testing.T) {
	macros := map[string]string{
		"personal": `"home" in tags`,
	}

	tests := []struct {
		name       string
		expression string
		macros     map[string]string
		env        map[string]any
		want       bool
	}{
		{
			name:       "include tag matches",
			expression: "+env",
			macros:     macros,
			env:        map[string]any{"tags": []string{"env", "dev"}},
			want:       true,
		},
		{
			name:       "exclude tag matches (should exclude)",
			expression: "!brew",
			macros:     macros,
			env:        map[string]any{"tags": []string{"brew", "dev"}},
			want:       false,
		},
		{
			name:       "combined include and exclude",
			expression: "+env !brew",
			macros:     macros,
			env:        map[string]any{"tags": []string{"env", "dev"}},
			want:       true,
		},
		{
			name:       "macro expansion matches",
			expression: "@personal",
			macros:     macros,
			env:        map[string]any{"tags": []string{"home"}},
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := compileExpr(tt.expression, tt.macros, true)
			if err != nil {
				t.Fatalf("compileExpr() unexpected error = %v", err)
			}

			got, err := evalCompiledExpr(program, tt.env)
			if err != nil {
				t.Fatalf("evalCompiledExpr() unexpected error = %v", err)
			}

			if got != tt.want {
				t.Errorf("evaluation result = %v, want %v", got, tt.want)
			}
		})
	}
}
