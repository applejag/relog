package main

import (
	"strings"
)

type PaddedString struct {
	maxWidth int
	history  []int
	index    int
}

func NewPaddedString(historyLen int) *PaddedString {
	if historyLen <= 0 {
		panic("NewPaddedString: history length must be positive")
	}
	return &PaddedString{
		history: make([]int, historyLen),
	}
}

func (p *PaddedString) Next(value string) string {
	removedWidth := p.history[p.index]
	p.index = (p.index + 1) % len(p.history)
	p.history[p.index] = len(value)
	if len(value) >= p.maxWidth {
		p.maxWidth = len(value)
		return value
	}
	if removedWidth < p.maxWidth {
		p.recalcMaxWidth()
	}
	return value + strings.Repeat(" ", p.maxWidth-len(value))
}

func (p *PaddedString) recalcMaxWidth() {
	p.maxWidth = p.history[0]
	for _, width := range p.history[1:] {
		if width > p.maxWidth {
			p.maxWidth = width
		}
	}
}
