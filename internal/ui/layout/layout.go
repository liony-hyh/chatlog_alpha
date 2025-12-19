package layout

import (
	"github.com/rivo/tview"
	"github.com/sjzar/chatlog/internal/ui/sidebar"
)

type Layout struct {
	*tview.Flex
	Sidebar *sidebar.Sidebar
	Pages   *tview.Pages
}

func New(s *sidebar.Sidebar, pages *tview.Pages) *Layout {
	l := &Layout{
		Flex:    tview.NewFlex(),
		Sidebar: s,
		Pages:   pages,
	}

	l.AddItem(s, 20, 0, true).
	  AddItem(pages, 0, 1, false)

	return l
}

func (l *Layout) FocusSidebar() {
	l.AddItem(l.Sidebar, 20, 0, true)
}
