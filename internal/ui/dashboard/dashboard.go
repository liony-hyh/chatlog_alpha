package dashboard

import (
	"fmt"

	"github.com/rivo/tview"
	"github.com/sjzar/chatlog/internal/ui/style"
)

type Dashboard struct {
	*tview.Table
}

func New() *Dashboard {
	d := &Dashboard{
		Table: tview.NewTable(),
	}

	d.SetBorders(false)
	d.SetBorder(true)
	d.SetTitle(" 状态概览 ")
	d.SetBorderColor(style.BorderColor)
	d.SetBackgroundColor(style.BgColor)
	
	return d
}

func (d *Dashboard) Update(data map[string]string) {
	d.Clear()
	
	row := 0
	headerColor := style.InfoBarItemFgColor
	
	keys := []string{
		"Account", "PID", "Status", "ExePath", 
		"Platform", "Version", "Session", "Data Key", 
		"Image Key", "Data Usage", "Data Dir", 
		"Work Usage", "Work Dir", "HTTP Server", "Auto Decrypt",
	}

	for _, key := range keys {
		val, ok := data[key]
		if !ok {
			continue
		}

		d.SetCell(row, 0, tview.NewTableCell(fmt.Sprintf(" [%s::b]%s", headerColor, key)).
			SetAlign(tview.AlignRight).
			SetExpansion(1).
			SetTextColor(style.FgColor))
			
		d.SetCell(row, 1, tview.NewTableCell(fmt.Sprintf(" %s", val)).
			SetAlign(tview.AlignLeft).
			SetExpansion(3).
			SetTextColor(style.FgColor))
			
		row++
	}
}
