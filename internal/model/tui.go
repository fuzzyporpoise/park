package model

import (
	"strings"

	"charm.land/lipgloss/v2"
	"go.fuzzyporpoise.dev/park/internal/config"
	"go.fuzzyporpoise.dev/park/internal/theme"
)

const (
	maxWidth      = 120
	defaultHeight = 20
)

type styles struct {
	doc               lipgloss.Style
	highlight         lipgloss.Style
	inactiveTab       lipgloss.Style
	activeTab         lipgloss.Style
	topBorder         lipgloss.Style
	window            lipgloss.Style
	errorText         lipgloss.Style
	helpText          lipgloss.Style
	submitButton      lipgloss.Style
	submitButtonFocus lipgloss.Style
	focusedPrompt     lipgloss.Style
	focusedText       lipgloss.Style
	blurredPrompt     lipgloss.Style
	blurredText       lipgloss.Style
	listNormalTitle   lipgloss.Style
	listNormalDesc    lipgloss.Style
	listSelectedTitle lipgloss.Style
	listSelectedDesc  lipgloss.Style
	listDimmedTitle   lipgloss.Style
	listDimmedDesc    lipgloss.Style
}

// fg builds a style with only a foreground color set. Most of the theme's
// styles are just this, so this collapses them from four lines to one.
func fg(color string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}

func newStyles() *styles {
	inactiveTabBorder := tabBorderWithBottom("┴", "─", "┴")
	activeTabBorder := tabBorderWithBottom("┘", " ", "└")

	s := new(styles)
	s.doc = lipgloss.NewStyle().
		Padding(1, 2)
	s.inactiveTab = lipgloss.NewStyle().
		Border(inactiveTabBorder, true).
		BorderForeground(lipgloss.Color(theme.CharmPink)).
		Foreground(lipgloss.Color(theme.CharmTextMute)).
		Padding(0, 1)
	s.activeTab = lipgloss.NewStyle().
		Border(activeTabBorder, true).
		BorderForeground(lipgloss.Color(theme.CharmPink)).
		Background(lipgloss.Color(theme.CharmPink)).
		Foreground(lipgloss.Color(theme.CharmBG)).
		Padding(0, 1)
	s.topBorder = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(lipgloss.Color(theme.CharmPink))
	s.highlight = fg(theme.CharmPink)
	s.window = lipgloss.NewStyle().
		BorderBottom(true).
		Padding(1, 2).
		Align(lipgloss.Center).
		UnsetBorderTop()
	s.errorText = fg(theme.CharmRed)
	s.helpText = fg(theme.CharmTextFaint)
	s.submitButton = fg(theme.CharmTextFaint)
	s.submitButtonFocus = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.CharmBG)).
		Background(lipgloss.Color(theme.CharmPink))
	s.focusedPrompt = fg(theme.CharmPink)
	s.focusedText = fg(theme.CharmText)
	s.blurredPrompt = fg(theme.CharmTextFaint)
	s.blurredText = fg(theme.CharmText)
	s.listNormalTitle = fg(theme.CharmText)
	s.listNormalDesc = fg(theme.CharmTextFaint)
	s.listSelectedTitle = fg(theme.CharmPurpleLt)
	s.listSelectedDesc = fg(theme.CharmTextMute)
	s.listDimmedTitle = fg(theme.CharmTextFaint)
	s.listDimmedDesc = fg(theme.CharmTextFaint)
	return s
}

// renderTabs renders a row of category tabs padded out to width with the
// top-border gap style, highlighting the tab at activeIdx. When focused is
// true, the active tab additionally gets a "❯ name ❮" indicator, used by
// the note form to show that the category selector itself has focus.
func (s *styles) renderTabs(categories []config.Category, activeIdx, width int, focused bool) string {
	var rendered []string
	for i, cl := range categories {
		style := s.inactiveTab
		label := cl.Name
		if i == activeIdx {
			style = s.activeTab
			if focused {
				label = "❯ " + cl.Name + " ❮"
			}
		}
		rendered = append(rendered, style.Render(label))
	}
	row := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
	gapWidth := max(0, width-lipgloss.Width(row))
	gap := s.topBorder.Render(strings.Repeat(" ", gapWidth))
	return lipgloss.JoinHorizontal(lipgloss.Bottom, row, gap)
}

// cycleIndex advances idx by delta and wraps around within [0, n). Used to
// cycle category selection in both the assist list and the note form.
func cycleIndex(idx, delta, n int) int {
	return ((idx+delta)%n + n) % n
}

func tabBorderWithBottom(left, middle, right string) lipgloss.Border {
	b := lipgloss.RoundedBorder()
	b.BottomLeft = left
	b.Bottom = middle
	b.BottomRight = right
	return b
}
