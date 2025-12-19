package sidebar

import (
	"github.com/rivo/tview"
	"github.com/sjzar/chatlog/internal/ui/style"
)

type Sidebar struct {
	*tview.List
}

func New(onSelected func(int, string, string, rune)) *Sidebar {
	s := &Sidebar{
		List: tview.NewList(),
	}

	s.ShowSecondaryText(false)
	s.SetBackgroundColor(style.BgColor)
	s.SetMainTextColor(style.FgColor)
	s.SetSelectedBackgroundColor(style.MenuBgColor)
	s.SetSelectedTextColor(style.PageHeaderFgColor)
	s.SetBorder(true)
	s.SetTitle(" 导航 ")
	s.SetTitleAlign(tview.AlignLeft)
	s.SetBorderColor(style.BorderColor)

	s.SetSelectedFunc(onSelected)

	return s
}

func (s *Sidebar) AddItem(text string, shortcut rune) {
	s.List.AddItem(text, "", shortcut, nil)
}
