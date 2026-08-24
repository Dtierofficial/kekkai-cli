package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Theme struct{ Name, Accent, Secondary, Muted, Soft, Background, Border, Key string }

var themes = []Theme{
	{"Fox Orange", "#FF8700", "#FF5F00", "#8A7568", "#E0D1C7", "#17131A", "#5A3A28", "#FF8700"},
	{"Cyberpunk Neon", "#FF007F", "#00FFFF", "#568A8A", "#B9FFFF", "#10141C", "#00FFFF", "#00FFFF"},
	{"Dracula", "#BD93F9", "#FF79C6", "#8F89A5", "#E6DDF4", "#282A36", "#514A68", "#BD93F9"},
	{"Matrix Green", "#00FF66", "#50FA7B", "#568A69", "#C8FFD7", "#0B120D", "#1C6334", "#50FA7B"},
	{"Deep Purple", "#9D4EDD", "#7B2CBF", "#806F91", "#DDC7EA", "#15101C", "#4C2869", "#9D4EDD"},
	{"Nordic Blue", "#00B4D8", "#80D8FF", "#62838D", "#D1F3FF", "#0D151A", "#265B6B", "#80D8FF"},
	{"Monochrome", "#FFFFFF", "#CCCCCC", "#888888", "#DDDDDD", "#111111", "#555555", "#FFFFFF"},
}

type userConfig struct {
	Theme string `json:"theme"`
}

func configPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "kekkai", "config.json"), nil
}
func loadTheme() Theme {
	t := themes[0]
	path, err := configPath()
	if err == nil {
		if b, e := os.ReadFile(path); e == nil {
			var c userConfig
			if json.Unmarshal(b, &c) == nil {
				for _, x := range themes {
					if strings.EqualFold(x.Name, c.Theme) {
						t = x
						break
					}
				}
			}
		}
	}
	applyTheme(t)
	return t
}
func saveTheme(t Theme) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(userConfig{Theme: t.Name})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(b)
	}
	if e := tmp.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
func applyTheme(t Theme) {
	fox = lipgloss.NewStyle().Foreground(lipgloss.Color(t.Accent)).Bold(true)
	coral = lipgloss.NewStyle().Foreground(lipgloss.Color(t.Secondary))
	muted = lipgloss.NewStyle().Foreground(lipgloss.Color(t.Muted))
	soft = lipgloss.NewStyle().Foreground(lipgloss.Color(t.Soft))
	selected = lipgloss.NewStyle().Foreground(lipgloss.Color(t.Background)).Background(lipgloss.Color(t.Accent)).Bold(true).Padding(0, 1)
	panel = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(t.Border)).Padding(1, 2)
	keyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.Key)).Bold(true)
}
func nextTheme(current Theme) Theme {
	for i, t := range themes {
		if t.Name == current.Name {
			n := themes[(i+1)%len(themes)]
			applyTheme(n)
			return n
		}
	}
	applyTheme(themes[0])
	return themes[0]
}
